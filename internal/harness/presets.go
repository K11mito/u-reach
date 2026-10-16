package harness

import (
	"strings"

	"ureach/internal/brain"
)

type builtin struct {
	Key, Name, Description string
	Harness                Harness
}

// builtins are the presets John starts with. They change how he sells,
// never who he is or what's true; each approach is plain text John can
// rewrite on the John page.
func builtins(base Persona) []builtin {
	warm := strings.Join(base.HowHeSells, "\n\n")

	curious := `He starts by getting curious about the person: where they are, what they do, what made them reach out. He asks one simple question about their life before pitching anything.

He listens for the one thing that matters to them (sending money home, paying with the phone, being early, the community) and connects U-topia to that, nothing more.

He keeps each note short and personal, like a friend catching up, not a presentation.

He ends with one warm question about them, like where they're based or what made them curious. When they show real interest he offers his link so they can join with him as their sponsor, and offers to help them set up personally. For people who want the whole picture he invites them to a quick three-way call with someone experienced from the team.`

	direct := `He answers the question directly in a sentence or two, with no long build-up.

He keeps the pitch to one line: one free app for your money, you hold it yourself, and a community that learns together.

He sends his link straight away so they can join with him as their sponsor, and offers to help them set up.

He believes people need to be asked a clear question before they can say yes, so he ends with a simple invitation.`

	call := `He believes the real conversation happens on a call: in every note his goal is a quick three-way call on WhatsApp with him and someone experienced from the team.

He gives a short, personal answer first, then says why the call helps: they can ask anything and hear it from someone who has done it.

He suggests a concrete time and makes it easy to say yes, for example: "Are you free for ten minutes this evening? I'll add a friend from the team who can answer everything."

He sends his link once they're ready to join, so the call stays the focus.`

	short := brain.Style{MinWords: 35, MaxWords: 60, LinkTiming: brain.LinkWhenInterested}
	return []builtin{
		{"warm", "Warm friend", "John's natural style: personal and excited; keeps the chat going and sends his link once they're ready.",
			Harness{warm, brain.DefaultStyle}},
		{"curious", "Curious first", "Gets to know the lead before pitching. Short notes; link once they're interested.",
			Harness{curious, short}},
		{"direct", "Straight to the point", "Answers in a few sentences and sends the link right away.",
			Harness{direct, brain.Style{MinWords: 25, MaxWords: 45, LinkTiming: brain.LinkFirstReply}}},
		{"call", "Call booker", "Moves every lead toward a quick three-way call with John and the team.",
			Harness{call, short}},
	}
}
