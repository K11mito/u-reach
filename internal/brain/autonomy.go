package brain

import "regexp"

// Autonomy is how much John's AI handles on its own before handing a chat to
// the real John. It's the slider on the John page. Each level changes what
// the prompt tells the model to hand off and the code net that forces a
// handoff when the model doesn't.
const (
	AutonomyLow    = "low"    // cautious: answers simple questions, hands off money, safety and anything unclear
	AutonomyMedium = "medium" // balanced: answers almost everything, hands off only when truly stuck (the default)
	AutonomyHigh   = "high"   // full: handles complaints and gaps too, hands off only extreme situations
)

// NormalizeAutonomy maps anything unknown to the balanced default.
func NormalizeAutonomy(a string) string {
	switch a {
	case AutonomyLow, AutonomyHigh:
		return a
	}
	return AutonomyMedium
}

// Level describes one autonomy level for the dashboard, so what John reads
// there matches what the code enforces.
type Level struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Summary   string   `json:"summary"`
	AIHandles []string `json:"ai_handles"`
	HandsOff  []string `json:"hands_off"`
}

// Levels, from most cautious to most autonomous.
var Levels = []Level{
	{
		Key: AutonomyLow, Label: "Cautious",
		Summary:   "Answers only simple questions and hands you money, safety and anything it isn't sure of.",
		AIHandles: []string{"Greetings and small talk", "What U-topia and U.Center are", "Questions the knowledge base answers directly"},
		HandsOff:  []string{"Prices, payments, earnings and rewards", "Safety, scam or pyramid questions", "Legal, tax and regulation", "Doubts, worries and complaints", "Wanting a call, or ready to join", "Anything the knowledge base doesn't cover", "Money gone wrong, legal threats or distress"},
	},
	{
		Key: AutonomyMedium, Label: "Balanced",
		Summary:   "Answers almost everything itself, honestly, and hands you only what it can't fix or answer safely.",
		AIHandles: []string{"Prices, packages, rewards and earnings", "Risks and whether it's legit", "Sending the link and booking calls"},
		HandsOff:  []string{"Payments, refunds or accounts gone wrong", "Legal, tax and regulatory advice", "Complaints, or asking for John himself", "Questions the knowledge base can't answer safely", "Legal threats, emergencies or distress"},
	},
	{
		Key: AutonomyHigh, Label: "Full autonomy",
		Summary:   "Handles the whole conversation, complaints included, and only calls you in extreme situations.",
		AIHandles: []string{"Everything Balanced answers", "Complaints, doubts and objections", "Gaps in the knowledge base, offering a call"},
		HandsOff:  []string{"Payments, refunds or accounts gone wrong", "Legal threats, police, fraud or theft", "Emergencies or someone in distress"},
	},
}

// LevelOf returns the description of an autonomy level.
func LevelOf(a string) Level {
	a = NormalizeAutonomy(a)
	for _, l := range Levels {
		if l.Key == a {
			return l
		}
	}
	return Levels[1]
}

// The code nets: words in the lead's message that force a handoff whatever
// the model wrote. The model is told the same; these are for when it doesn't
// listen.
var (
	// Every level: money that went wrong, legal trouble, someone in danger.
	handoffRe = regexp.MustCompile(`(?i)\b(refund\w*|chargebacks?|lawsuits?|lawyers?|attorneys?|sue|suing|police|hacked|scammed|defrauded|stole|stolen|theft|lost (my|all|the|our) (money|savings|funds)|suicid\w*|kill (myself|me)|emergency)\b`)
	// Balanced and Cautious: complaints, or asking for a real person.
	personRe = regexp.MustCompile(`(?i)\b(complain\w*|real (john|person|human)|human being|(speak|talk) (to|with) (a )?(real |live )?(person|human))\b`)
	// Cautious only: money, safety and trust topics John wants for himself.
	cautiousRe = regexp.MustCompile(`(?i)\b(earn\w*|income|commissions?|profits?|returns?|roi|passive|guarantee\w*|withdraw\w*|payouts?|salary|legal|illegal|scam|ponzi|pyramid|tax(es)?|regulat\w*|safe|safety|secure|risk\w*|trust\w*|fees?|costs?|prices?|pricing|pay|paid|paying|payments?|money back|angry|upset|worried|scared|meet|call me)\b`)
)

// handoffTrigger is the word in the lead's message that must go to John at
// this autonomy level, or "". The levels are nested: whatever a bolder level
// hands to John, a more cautious one does too.
func handoffTrigger(autonomy, text string) string {
	nets := []*regexp.Regexp{handoffRe, personRe, cautiousRe}
	switch NormalizeAutonomy(autonomy) {
	case AutonomyHigh:
		nets = nets[:1]
	case AutonomyMedium:
		nets = nets[:2]
	}
	for _, re := range nets {
		if kw := re.FindString(text); kw != "" {
			return kw
		}
	}
	return ""
}

const (
	honestyRule = `Honesty, always: never promise or estimate income, earnings or returns, and never say what anyone has earned. Never call the membership an investment or say what points or a membership might be worth. Never say U-topia is a bank, regulated or licensed, and never give a legal verdict like "it's legal" or "it's not a pyramid"; describe how it works and let them decide.`
	holdingRule = `When you hand off, spoken_script is only a short, warm holding line (20 to 40 words) saying John will get back to them personally. Do not answer the question. Put the topic in handoff_reason.`
)

// handoffPrompt tells the model what to answer itself and when to hand a
// chat to John.
func handoffPrompt(autonomy string) string {
	switch NormalizeAutonomy(autonomy) {
	case AutonomyLow:
		return `Be careful: John wants to handle most conversations himself. Only answer greetings and simple questions the knowledge base answers directly, like what U-topia or U.Center is.

` + honestyRule + `

Set handoff to true for everything else: earnings, rewards, commissions or returns; prices, fees, packages or how to pay; refunds or withdrawals; anything legal, regulatory or tax; whether it's safe, a scam or a pyramid; any doubt, worry, objection or complaint; anything personal or emotional; when they want to meet or talk on a call; when they're ready to join; and anything the knowledge base and playbook don't clearly answer. When in doubt, hand off.
` + holdingRule
	case AutonomyHigh:
		return `Answer, don't defer. John trusts you with the whole conversation, end to end: prices, packages, rewards and earnings, risks, whether it's legit, doubts, objections and complaints. Answer them yourself the way the knowledge base says to talk about them. If someone is upset, stay calm, listen and help. If a detail isn't covered, say what you do know honestly and offer a quick call.

` + honestyRule + `

Handoff is only for extreme situations: set it to true only when
1. Money or an account went wrong and only a person can fix it: a payment, purchase or transfer that didn't arrive, a refund, lost money or lost access.
2. They make legal threats (lawyers, police, regulators) or accuse John or U-topia of fraud or theft.
3. There's a threat, an emergency, or someone is in distress.
` + holdingRule
	}
	return `Answer, don't defer. The whole point is to save John time, so you answer questions yourself, including prices, packages, rewards and earnings, risks, and whether it's legit, the way the knowledge base says to talk about them.

` + honestyRule + `

Handoff is rare: set it to true only when you're truly stuck.
1. Something only a person can fix: a payment, purchase, transfer or account that went wrong, a refund, or lost access.
2. They want legal, tax or regulatory advice, or ask whether it's allowed where they live.
3. They're upset or complaining, or ask to speak to John himself.
4. They ask something important the knowledge base can't answer and a guess could mislead them, like whether the card works in their specific country.
5. There's a legal threat, an emergency, or someone is in distress.
When you hand off, still help: say what you can, warmly, then say John will get back to them personally on the rest (20 to 50 words). Put the topic in handoff_reason.`
}
