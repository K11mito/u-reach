package brain

import "regexp"

// Autonomy is how much John's AI handles on its own before handing a chat to
// the real John. It's the slider on the John page. Each level changes what
// the prompt tells the model to hand off and the code net that forces a
// handoff when the model doesn't.
const (
	AutonomyLow    = "low"    // cautious: answers simple questions, hands off anything sensitive or unclear
	AutonomyMedium = "medium" // balanced: hands off money, legal and trust topics (the original rules)
	AutonomyHigh   = "high"   // full: handles almost everything, hands off only extreme situations
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
		Summary:   "Answers only simple questions and hands you anything sensitive or unclear.",
		AIHandles: []string{"Greetings and small talk", "What U-topia and U.Center are", "Questions the facts answer directly"},
		HandsOff:  []string{"Prices, payments, earnings and refunds", "Safety, scam or pyramid questions", "Doubts, worries and complaints", "Wanting to meet or get a call", "Leads who are ready to join", "Anything the facts don't cover", "Legal threats, emergencies or distress"},
	},
	{
		Key: AutonomyMedium, Label: "Balanced",
		Summary:   "Sells and answers questions, and hands you money, legal and trust topics.",
		AIHandles: []string{"Everything in the facts and playbook", "Prices, packages and how to join", "Sending the link and booking calls"},
		HandsOff:  []string{"Earnings, income and commissions", "Returns, profits and guarantees", "Payments, refunds and withdrawals", "Legal, tax and regulation", "Scam or pyramid questions", "Threats, emergencies or someone in distress"},
	},
	{
		Key: AutonomyHigh, Label: "Full autonomy",
		Summary:   "Handles the whole conversation and only calls you in extreme situations.",
		AIHandles: []string{"Prices, packages and how to pay", "Doubts, objections, scam or pyramid questions", "Earnings questions, without ever promising income"},
		HandsOff:  []string{"Legal threats, lawyers or regulators", "Accusations of fraud or theft", "Lost money, refunds or withdrawal problems", "Threats, emergencies or someone in distress"},
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

var (
	// Extra lead topics a cautious John wants for himself (on top of handoffRe).
	cautiousRe = regexp.MustCompile(`(?i)\b(safe|safety|secure|risk\w*|trust\w*|lost money|hack\w*|fees?|costs?|prices?|pricing|pay|paid|paying|payments?|money back|complain\w*|angry|upset|worried|scared|meet|call me)\b`)
	// The only lead topics a fully autonomous John still hands over.
	extremeRe = regexp.MustCompile(`(?i)\b(lawsuit|lawyers?|attorney|sue|suing|police|regulators?|fraud\w*|scammed|stole|stolen|theft|refund\w*|chargebacks?|withdraw\w*|lost (my|all|the|our) (money|savings|funds)|suicid\w*|kill (myself|me)|emergency)\b`)
	// Income claims in the AI's own reply. Never sent, at any level: an
	// autonomous John must not drift into promises.
	promiseRe = regexp.MustCompile(`(?i)\b(passive income|guaranteed? (returns?|income|profits?|earnings)|(earn|make)\s+(up to |around |about |over |at least )?(\$\s?\d|\d[\d,.]*\s*(k\b|dollars|usd|usdt|baht|euros?|per |a (month|week|day))|(a few |several )?(hundreds|thousands|millions)\b|(one|two|three|four|five|six|seven|eight|nine|ten|twenty|fifty) (hundred|thousand|million)))`)
)

// handoffTrigger is the word in the lead's message that must go to John at
// this autonomy level, or "". The levels are nested: whatever a bolder level
// hands to John, a more cautious one does too.
func handoffTrigger(autonomy, text string) string {
	if kw := extremeRe.FindString(text); kw != "" {
		return kw
	}
	switch NormalizeAutonomy(autonomy) {
	case AutonomyHigh:
		return ""
	case AutonomyLow:
		if kw := handoffRe.FindString(text); kw != "" {
			return kw
		}
		return cautiousRe.FindString(text)
	}
	return handoffRe.FindString(text)
}

const holdingRule = ` Then spoken_script is only a short, warm holding line (20 to 40 words) saying John will get back to them personally. Do not answer the question. Put the topic in handoff_reason.`

// handoffPrompt tells the model when to hand a chat to John.
func handoffPrompt(autonomy string) string {
	switch NormalizeAutonomy(autonomy) {
	case AutonomyLow:
		return `Handoff (John wants to handle most conversations himself): only answer greetings and simple questions the FAQ answers directly, like what U-topia or U.Center is. Set handoff to true for everything else: earnings, income, commissions or returns; prices, fees, packages or how to pay; refunds or withdrawals; anything legal, regulatory or tax; whether it's safe, a scam or a pyramid; any doubt, worry, objection or complaint; anything personal or emotional; when they want to meet or talk on a call; when they're ready to join; and anything the FAQ and playbook don't clearly answer. When in doubt, hand off.` + holdingRule
	case AutonomyHigh:
		return `Handoff (John trusts you to handle the conversation end to end): answer questions about prices, packages and how to pay from the FAQ. Handle doubts, worries and objections yourself, calmly and honestly, including whether it's safe, a scam or a pyramid: explain from the FAQ what U-topia and U.Center are, with no hype. If they ask about earnings, never promise, estimate or hint at any income or returns; say it depends on the person and what they put in, and invite them to a quick call. Only set handoff to true in extreme situations: legal threats, lawyers or regulators; accusations of fraud or theft; money they say they lost, or a refund or withdrawal problem; threats, emergencies or someone in distress.` + holdingRule
	}
	return `Handoff: if the lead asks about earnings, income, commissions, returns, payments or withdrawals, anything legal or regulatory, complaints, or whether it's a scam or pyramid, set handoff to true.` + holdingRule
}
