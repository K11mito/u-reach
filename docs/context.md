# U-topia, U.Center and how ambassadors talk to leads

Distilled from `transcripts/` (community calls Aug to Oct 2026, Emmanuel's explainers, John's talk). Timestamps and quotes are in the transcripts; this is the short version.

## The two things
- **U-topia** is the company and its app: a free, all-in-one money app (exchange + wallet + Wise-style transfers + bank-style account). It's non-custodial ("I'm not touching your money"), not a bank, no interest or credit. You send money to a name (U-tag), pay with your phone wherever Visa works, and hold currencies, stablecoins, Bitcoin and tokenized gold. It works in 50+ countries at launch, with physical cards after launch.
  - Members got the app on **Oct 15**. It opens to everyone on **Oct 16 at the Flow Summit**, Avani Sukhumvit, Bangkok. That's this event; **U-Hackathon runs alongside it**.
- **U.Center** is the paid community behind the app: "an education DAO" and "the ambassador app". U-topia grows by word of mouth through it instead of ads.

## How the money side works (network marketing)
- You join U.Center by buying a **one-time lifetime package** through your **sponsor's referral link**.
  - Packages (u.center): Starter $25, Bronze $100, Silver $250, Gold $500, Platinum $1,000, Diamond $2,500, plus Full House/"superstar" (~$4,350, all packages bundled).
  - You pay in **USDT on BNB Chain**.
- **Ranks ("Flow 1 ... Flow 20")** go up with qualifying volume from your **downline** (people you refer, and the people they refer), with a two-leg rule.
- Rewards come from the **wealth pool**: 19 pools of commissions shared out by rank. You also earn **direct commissions** on referrals.
- **DAO points ("dows")** and promised **ownership/stake** in U-topia sit on top of that.
- The calls talk about all of this freely, with almost no disclaimers. Examples: "an investment for your grandkids", "massive stake position", "scarcity event".
- **The bot answers these itself, honestly.** It explains how rewards work without ever promising income, calling the membership an investment, or giving legal verdicts (`data/faq.md`, "Rewards for sharing" and "Is it legit?"). It only hands off when truly stuck: money or accounts gone wrong, legal or tax advice, upset leads, or questions the knowledge base can't answer safely. A code net also blocks any reply that promises money.
- That is the default ("Balanced") on the **autonomy slider** on the John page. "Cautious" also hands off prices, earnings, safety and doubts; "Full autonomy" handles complaints and gaps too and only hands off money gone wrong, legal threats and emergencies. Levels are nested, and the income net applies at every level (`internal/brain/autonomy.go`).

## How an ambassador conversation goes (John's three-way call)
1. An ambassador meets a **prospect** (WhatsApp, Zoom, or a cafe table).
2. They set up a **three-way call** with their **sponsor**. The ambassador **edifies** the sponsor ("great leader, very knowledgeable"). The sponsor edifies the ambassador back. "It's the power of the third party."
3. The sponsor gives a **short pitch that is always the same**.
4. The next steps are a group **discovery call** (Zach and George on Tue/Thu; Dirk on Sundays in German), then the **referral link**, buying a package, and Academy onboarding.
5. The new **enrollee** does 10-12 three-way calls with their sponsor in the first week until they can do it alone. This is **duplication**, and it "becomes culture".

John has done this for 20+ years and does most calls on WhatsApp. That's the hook for U-Reach: his voice can answer the first questions instantly, and he steps in for the three-way call.

## Who's who
- **Emmanuel Quezada:** founder and CEO. Gives the launch keynote on Oct 16.
- **Kyle Kemper:** president of U-topia, and our client. On the 26 Sept call he described wanting an AI agent that finds "the top 100 people who might be interested in joining U Center based on all my messages", invites them, and answers replies. That's U-Reach.
- **Maya:** CTO and creator of **U Bot**. She is a **hackathon judge**. Hackathon teams were told they'd build "on top of U Bot".
- **Zach, George:** discovery calls and community calls. **Dirk:** German calls. **Sahib:** head of product.
- **John Klensch:** ambassador who teaches the three-way call.

## Open questions for Kyle
- Does the hackathon require or reward building on **U Bot**? If so, how do we plug in?
- John's real **referral link**, and whether replies must disclose they're AI-generated.
- Should the bot ever mention the **Flow Summit** or discovery calls (days and times change)?
- Which languages do John's leads use? This decides the TTS model.
