"""Single source of truth for SELAR marketing copy. Every claim here must respect the
research-integrity rules: research prototype; no memory/retention/grade/learning-gain claims;
survey numbers exactly as reported (38 eligible respondents; item denominators 36)."""

APP_URL = "https://selar-console.vercel.app"
APP_URL_SHORT = "selar-console.vercel.app"
TEAM = "UCSC IS4101 Group 04"
MEMBERS = ["G.P.G.S. Ganegoda", "H.K.S.R. Hapuarachchi", "K.A.D.A.A. Kuruppu Arachchi"]
SUPERVISOR = "Dr. Thushani A. Weerasinghe"
PRINCIPLE = "AI suggests, sources show, you explain and decide."
FULL_NAME = "Semantic Linking for Active Retention"
STATUS = "Research prototype"

TAGLINES = [
    ("AI suggests. Sources show. You decide.", "Recommended. It is the product principle in six words, so it stays true as features change, and it answers the AI-accuracy worry up front."),
    ("Every new page has an old friend.", "The playful one. Good for posters, stickers and Linny's speech bubbles."),
    ("Spot the link. Say why. Keep what holds.", "Describes the actual loop: notice, explain, decide."),
    ("Links you can check, not just trust.", "For audiences wary of AI; leads with the side-by-side sources."),
    ("Your reading, connected by you.", "Short, warm, puts the reader in charge."),
    ("Read. Link. Explain. Decide.", "A four-beat version of the workflow for slides and section headers."),
    ("The reading buddy that asks before it assumes.", "Mascot-led line for onboarding and social posts."),
    ("Old notes, new reading, one honest link.", "A quieter option for academic settings."),
]

PITCH = (
    "You're reading something new and it quietly connects to something you read weeks ago, but the thought slips away. "
    "SELAR is a research prototype PDF reader from UCSC that catches it. It suggests a possible link to an earlier passage, "
    "shows both sources side by side, then hands the thinking back to you: explain it, compare, and keep, change or reject the link. "
    "AI suggests, sources show, you decide. Try it and tell us what works."
)

PITCH_LONG = (
    "You read something new, and it quietly connects to something you read weeks ago. Often that thought slips away before you do anything with it. "
    "SELAR is a research prototype PDF reader from the University of Colombo School of Computing. While you read, it suggests a possible "
    "connection to an earlier passage and puts both passages side by side. Then it hands the thinking back to you: explain the link, compare the two, "
    "and decide whether to keep it, change it or reject it. The design draws on learning research on depth of processing, self-explanation, "
    "comparison and retrieval practice. In our formative survey, 31 of 36 respondents wanted a tool that links new and earlier reading, and 28 of 36 were "
    "worried about AI accuracy, which is why every suggestion shows its sources. We are not claiming it improves memory or grades; a planned study will look at that. "
    "Right now we want people to try it and tell us what works."
)

SURVEY = [
    ("31 of 36", "wanted a tool that links new reading to earlier reading"),
    ("28 of 36", "were concerned about AI accuracy"),
    ("24 of 36", "were concerned about doing less active thinking"),
]
SURVEY_NOTE = "Formative needs survey, 38 eligible respondents; 36 answered these items. Not an evaluation of SELAR."

STEPS = [
    ("Read", "Open your PDFs and read as usual."),
    ("Notice", "SELAR suggests a possible link to something you read earlier."),
    ("Compare", "Both source passages appear side by side, so you can check the link yourself."),
    ("Explain & decide", "Say how they connect in your own words, then keep, change or reject the link."),
]

GROUNDING = "Grounded in learning research on depth of processing, self-explanation, comparison and retrieval practice."

CAROUSEL = [
    {
        "kicker": "SELAR · research prototype",
        "title": "Ever read something and think, “wait, I’ve seen this before”?",
        "body": "That flash of connection is easy to lose. SELAR is a PDF reader we are building to catch it, and then hand it back to you.",
        "pose": "thinking",
    },
    {
        "kicker": "How it works",
        "title": "AI suggests. Sources show. You decide.",
        "body": None,
        "steps": True,
        "pose": "linking",
    },
    {
        "kicker": "Why we built it this way",
        "title": "You asked for links, and for honesty about AI.",
        "body": None,
        "survey": True,
        "pose": "questioning",
    },
]

CAPTIONS = {
    "carousel": (
        "Ever read a paper and think \"wait, this connects to something I read last month\"? Then the thought is gone.\n\n"
        "We are building SELAR, a research prototype PDF reader, to catch that moment. While you read, it suggests a possible link "
        "to an earlier passage and shows both sources side by side. Then it asks you to explain the link and decide: keep it, change it, or reject it.\n\n"
        "The AI only suggests. You do the thinking.\n\n"
        "We designed it around what students told us in our needs survey: most wanted help linking new and earlier reading, "
        "and most also worried about AI getting things wrong. So every suggestion comes with its sources.\n\n"
        "It is an early prototype from our final-year research at UCSC, and feedback is very welcome.\n\n"
        "#LearningSciences #EdTech #HumanCenteredAI #ResearchPrototype"
    ),
    "square-principle": (
        "Our design rule for SELAR fits on a sticky note: AI suggests, sources show, you explain and decide.\n\n"
        "A suggested link is only a starting point. You see both passages, and the link only stays if you can say why it holds.\n\n"
        "SELAR is a research prototype from UCSC IS4101 Group 04. Feedback welcome.\n\n"
        "#HumanCenteredAI #EdTech #ActiveLearning"
    ),
    "square-linny": (
        "Meet Linny, the small linnet who carries pages between what you are reading now and what you read before.\n\n"
        "Linny will point out a possible link, but it is your call whether it holds. Sometimes the right answer is \"no, that's a stretch\", "
        "and rejecting a weak link counts as good thinking too.\n\n"
        "SELAR is a research prototype. Try it and tell us what you think.\n\n"
        "#EdTech #LearningSciences #ResearchPrototype"
    ),
}

VOICE_DO_DONT = [
    ("Spotted a possible link to Chapter 2. Does it hold?", "We found the perfect connection for you!"),
    ("SELAR is a research prototype. Feedback welcome.", "SELAR boosts your memory by 40%."),
    ("Grounded in research on self-explanation and retrieval practice.", "Scientifically proven to improve grades."),
    ("Not sure? Reject it. Weak links are worth catching.", "Error: invalid link rejected."),
    ("Nothing linked yet. Read a little more and Linny will check back.", "No data. Upload content to continue."),
]

CLAIM_RULES = [
    "Always say \"research prototype\" somewhere visible.",
    "Never claim SELAR improves memory, retention, grades or learning. The retention study is planned, not run.",
    "You may say the design is grounded in learning research (depth of processing, self-explanation, comparison, retrieval practice).",
    "Quote the survey only as: 38 eligible respondents; 31/36 wanted linking; 28/36 worried about AI accuracy; 24/36 worried about less active thinking.",
    "No testimonials, user counts, partner logos, prices or incentives.",
    "Do not promise anything about a study (participation, payment, results) until ethics approval is confirmed.",
]
