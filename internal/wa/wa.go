// Package wa wraps whatsmeow: session, QR linking, filtered inbound messages,
// text and voice-note sending, and the "recording audio..." presence.
package wa

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

// Inbound is a direct message from a lead, after filtering.
type Inbound struct {
	ID        types.MessageID
	Chat      types.JID // reply here (may be a LID JID)
	Sender    types.JID // raw sender, needed for read receipts
	From      types.JID // phone-number JID when we can resolve it, else the LID
	Phone     string    // digits only; empty if only a LID is known
	PushName  string
	Text      string
	Audio     *waE2E.AudioMessage // set for voice notes and audio files
	Timestamp time.Time
}

// Client is a linked WhatsApp companion device.
type Client struct {
	WA *whatsmeow.Client

	// OnMessage runs in its own goroutine per message; it may block.
	OnMessage func(Inbound)

	log       waLog.Logger
	lock      *os.File
	startedAt time.Time
	allow     map[string]bool

	connOnce  sync.Once
	connected chan struct{}
}

// MaxBacklog is how old a message may be (relative to process start) and still
// get answered. Older ones are replayed backlog from while we were offline.
const MaxBacklog = 2 * time.Minute

// Open loads (or creates) the session at sessionPath. Only one process may hold
// a session at a time; a second one would kick the first off WhatsApp.
func Open(ctx context.Context, sessionPath string, allowlist []string, log waLog.Logger) (*Client, error) {
	lock, err := os.OpenFile(sessionPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("session %s is in use by another process (wa-spike or server?)", sessionPath)
	}

	dsn := "file:" + sessionPath + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	container, err := sqlstore.New(ctx, "sqlite", dsn, log.Sub("DB"))
	if err != nil {
		lock.Close()
		return nil, err
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		lock.Close()
		return nil, err
	}

	c := &Client{
		WA:        whatsmeow.NewClient(device, log.Sub("Client")),
		log:       log,
		lock:      lock,
		startedAt: time.Now(),
		allow:     map[string]bool{},
		connected: make(chan struct{}),
	}
	for _, p := range allowlist {
		if p = digits(p); p != "" {
			c.allow[p] = true
		}
	}
	c.WA.AddEventHandler(c.handle)
	return c, nil
}

// Connect connects, linking first if the session is new. With pairPhone set
// (international format, digits), it prints an 8-character pairing code instead
// of a QR code: WhatsApp > Linked devices > Link with phone number.
func (c *Client) Connect(ctx context.Context, pairPhone string) error {
	if c.WA.Store.ID != nil {
		if err := c.WA.Connect(); err != nil {
			return err
		}
		return c.waitConnected(ctx)
	}

	qrs, err := c.WA.GetQRChannel(ctx)
	if err != nil {
		return err
	}
	if err := c.WA.Connect(); err != nil {
		return err
	}
	pairing := false
	for item := range qrs {
		switch item.Event {
		case "code":
			if pairPhone != "" {
				if !pairing {
					pairing = true
					code, err := c.WA.PairPhone(ctx, digits(pairPhone), true, whatsmeow.PairClientChrome, "Chrome (Linux)")
					if err != nil {
						return fmt.Errorf("pair phone: %w", err)
					}
					fmt.Printf("\nPairing code: %s\nOn the demo phone: WhatsApp > Settings > Linked devices > Link a device > Link with phone number instead\n\n", code)
				}
				continue
			}
			fmt.Println("\nScan with the demo phone: WhatsApp > Settings > Linked devices > Link a device")
			qrterminal.GenerateHalfBlock(item.Code, qrterminal.L, os.Stdout)
		case "success":
			fmt.Println("Linked. Session saved; restarts won't need a QR.")
			return c.waitConnected(ctx)
		case "timeout":
			return errors.New("QR timed out: run again")
		default:
			if item.Error != nil {
				return fmt.Errorf("pairing %s: %w", item.Event, item.Error)
			}
			c.log.Infof("pairing event: %s", item.Event)
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("QR channel closed before linking")
}

func (c *Client) waitConnected(ctx context.Context) error {
	select {
	case <-c.connected:
		return nil
	case <-time.After(45 * time.Second):
		return errors.New("timed out waiting for WhatsApp connection")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close disconnects and releases the session lock.
func (c *Client) Close() {
	c.WA.Disconnect()
	if c.lock != nil {
		c.lock.Close()
	}
}

func (c *Client) handle(evt any) {
	switch e := evt.(type) {
	case *events.Connected:
		c.log.Infof("connected as %s", c.WA.Store.ID)
		c.connOnce.Do(func() { close(c.connected) })
		go c.announceAvailable()
	case *events.LoggedOut:
		c.log.Errorf("LOGGED OUT (reason %v): delete %s and link again", e.Reason, "the session DB")
	case *events.StreamReplaced:
		c.log.Errorf("another client took over this session; is a second process running?")
	case *events.Message:
		if in, ok := c.filter(e); ok && c.OnMessage != nil {
			go c.OnMessage(in) // never block whatsmeow's event loop
		}
	}
}

// announceAvailable marks us online, which chat presence (recording...) needs.
// Right after linking the push name hasn't synced yet, so retry for a while.
func (c *Client) announceAvailable() {
	for range 30 {
		err := c.WA.SendPresence(context.Background(), types.PresenceAvailable)
		if err == nil {
			c.log.Infof("presence: available (push name %q)", c.WA.Store.PushName)
			return
		}
		if !errors.Is(err, whatsmeow.ErrNoPushName) {
			c.log.Warnf("send presence: %v", err)
			return
		}
		time.Sleep(2 * time.Second)
	}
	c.log.Warnf("push name never synced; recording indicator may not show. Set a profile name on the demo phone.")
}

func (c *Client) filter(e *events.Message) (Inbound, bool) {
	info := e.Info
	switch {
	case info.IsFromMe:
		return Inbound{}, false
	case info.IsGroup, info.Chat.Server == types.BroadcastServer, info.Chat.Server == types.NewsletterServer:
		return Inbound{}, false
	case info.Timestamp.Before(c.startedAt.Add(-MaxBacklog)):
		c.log.Infof("skip backlog message %s from %s (%s old)", info.ID, info.Sender, time.Since(info.Timestamp).Round(time.Second))
		return Inbound{}, false
	}

	from := c.phoneJID(info.MessageSource)
	in := Inbound{
		ID:        info.ID,
		Chat:      info.Chat,
		Sender:    info.Sender,
		From:      from,
		PushName:  info.PushName,
		Timestamp: info.Timestamp,
	}
	if from.Server == types.DefaultUserServer {
		in.Phone = from.User
	}

	msg := e.Message
	switch {
	case msg.GetConversation() != "":
		in.Text = msg.GetConversation()
	case msg.GetExtendedTextMessage().GetText() != "":
		in.Text = msg.GetExtendedTextMessage().GetText()
	case msg.GetAudioMessage() != nil:
		in.Audio = msg.GetAudioMessage()
	default:
		return Inbound{}, false // reactions, edits, stickers, receipts-like protocol messages
	}

	if len(c.allow) > 0 && !c.allow[in.Phone] {
		c.log.Infof("skip %s (%s): not on ALLOWLIST", in.From, in.PushName)
		return Inbound{}, false
	}
	return in, true
}

// phoneJID resolves the sender to a phone-number JID. WhatsApp increasingly
// addresses DMs by LID; without this, a reply from a contact we messaged by
// phone number would look like a brand-new contact.
func (c *Client) phoneJID(src types.MessageSource) types.JID {
	s := src.Sender.ToNonAD()
	if s.Server == types.DefaultUserServer {
		return s
	}
	if alt := src.SenderAlt.ToNonAD(); alt.Server == types.DefaultUserServer {
		return alt
	}
	if s.Server == types.HiddenUserServer {
		if pn, err := c.WA.Store.LIDs.GetPNForLID(context.Background(), s); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD()
		}
	}
	return s
}

// SendText sends a plain text message.
func (c *Client) SendText(ctx context.Context, to types.JID, text string) (types.MessageID, error) {
	resp, err := c.WA.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(text)})
	return resp.ID, err
}

// SendVoiceNote uploads Ogg Opus audio and sends it as a push-to-talk bubble.
func (c *Client) SendVoiceNote(ctx context.Context, to types.JID, ogg []byte, seconds uint32, waveform []byte) (types.MessageID, error) {
	up, err := c.WA.Upload(ctx, ogg, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("upload: %w", err)
	}
	msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		URL:               proto.String(up.URL),
		DirectPath:        proto.String(up.DirectPath),
		MediaKey:          up.MediaKey,
		FileEncSHA256:     up.FileEncSHA256,
		FileSHA256:        up.FileSHA256,
		FileLength:        proto.Uint64(up.FileLength),
		Mimetype:          proto.String("audio/ogg; codecs=opus"),
		PTT:               proto.Bool(true), // without this it arrives as a music file
		Seconds:           proto.Uint32(seconds),
		Waveform:          waveform,
		MediaKeyTimestamp: proto.Int64(time.Now().Unix()),
	}}
	resp, err := c.WA.SendMessage(ctx, to, msg)
	return resp.ID, err
}

// Recording shows "recording audio..." in chat until the returned stop func is
// called. Clients drop the indicator after ~25 s, so it is refreshed.
func (c *Client) Recording(chat types.JID) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	send := func(state types.ChatPresence) {
		if err := c.WA.SendChatPresence(context.Background(), chat, state, types.ChatPresenceMediaAudio); err != nil {
			c.log.Warnf("chat presence %s: %v", state, err)
		}
	}
	send(types.ChatPresenceComposing)
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(8 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				send(types.ChatPresenceComposing)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
			send(types.ChatPresencePaused)
		})
	}
}

// MarkRead sends blue ticks for a message.
func (c *Client) MarkRead(ctx context.Context, in Inbound) {
	if err := c.WA.MarkRead(ctx, []types.MessageID{in.ID}, time.Now(), in.Chat, in.Sender); err != nil {
		c.log.Warnf("mark read: %v", err)
	}
}

// DownloadAudio fetches and decrypts an inbound voice note.
func (c *Client) DownloadAudio(ctx context.Context, a *waE2E.AudioMessage) ([]byte, error) {
	return c.WA.Download(ctx, a)
}

// PhoneJID builds a JID for an outbound message to a phone number.
func PhoneJID(phone string) types.JID {
	return types.NewJID(digits(phone), types.DefaultUserServer)
}

func digits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
