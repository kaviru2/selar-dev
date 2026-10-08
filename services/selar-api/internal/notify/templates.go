package notify

import (
	"fmt"
	"strings"
	"time"

	"github.com/selar-dev/selar-api/internal/calendar"
	"github.com/selar-dev/selar-api/internal/quiz"
)

// Kind is a notice type. Values are stored in notification_log.kind.
type Kind string

const (
	QuizOpening          Kind = "quiz_opening"
	QuizClosingSoon      Kind = "quiz_closing_soon"
	SecurityPassword     Kind = "security_password_changed"
	SecurityEmail        Kind = "security_email_changed"
	securityFooterReason      = "You are receiving this because you turned on account security emails in SELAR Settings."
	quizFooterReason          = "You are receiving this because you turned on quiz emails in SELAR Settings."
)

// Kinds lists every notice kind.
var Kinds = []Kind{QuizOpening, QuizClosingSoon, SecurityPassword, SecurityEmail}

// IsQuiz reports whether k belongs to the quiz notice family.
func (k Kind) IsQuiz() bool { return k == QuizOpening || k == QuizClosingSoon }

// TemplateData is everything a template may use. It deliberately has no
// field for quiz titles, cohorts, groups, scores or anything else that could
// differ between study arms.
type TemplateData struct {
	QuizKind       quiz.Kind
	Opens, Closes  *time.Time
	At             time.Time // when a security event happened
	ConsoleURL     string
	UnsubscribeURL string
	Location       *time.Location
}

func (d TemplateData) when(t time.Time) string {
	loc := d.Location
	if loc == nil {
		loc = time.UTC
	}
	name := loc.String()
	if name == "Local" {
		name = "server time"
	}
	return fmt.Sprintf("%s (%s time)", t.In(loc).Format("Mon 2 Jan 2006, 15:04"), name)
}

func footer(reason string, d TemplateData) string {
	var b strings.Builder
	b.WriteString("\n\n--\n")
	b.WriteString(reason)
	b.WriteString(" SELAR is a research prototype run by a student research team.")
	if d.UnsubscribeURL != "" {
		b.WriteString("\nTo stop all SELAR emails, open this link: " + d.UnsubscribeURL)
	}
	b.WriteString("\n")
	return b.String()
}

// Render builds the subject and plain-text body for a notice. Every
// participant gets the same wording for the same kind of notice.
func Render(k Kind, d TemplateData) (subject, body string, err error) {
	label := strings.ToLower(calendar.KindLabel(d.QuizKind))
	quizzes := calendar.QuizzesURL(d.ConsoleURL)
	switch k {
	case QuizOpening:
		subject = "SELAR: a quiz is open"
		body = "Hello,\n\nA SELAR " + label + " is now open for you."
		if d.Closes != nil {
			body += " You can take it until " + d.when(*d.Closes) + "."
		} else {
			body += " It has no closing time."
		}
		body += "\n\nYour quizzes: " + quizzes + footer(quizFooterReason, d)
	case QuizClosingSoon:
		if d.Closes == nil {
			return "", "", fmt.Errorf("closing notice needs a close time")
		}
		subject = "SELAR: a quiz closes soon"
		body = "Hello,\n\nA SELAR " + label + " that is open for you closes at " + d.when(*d.Closes) + "." +
			"\n\nYour quizzes: " + quizzes + footer(quizFooterReason, d)
	case SecurityPassword:
		subject = "SELAR: your password was changed"
		body = "Hello,\n\nThe password for your SELAR account was changed at " + d.when(d.At) + ". Other signed-in sessions were signed out." +
			"\n\nIf you made this change, you do not need to do anything. If you did not, please contact the SELAR research team." +
			footer(securityFooterReason, d)
	case SecurityEmail:
		subject = "SELAR: your sign-in email was changed"
		body = "Hello,\n\nThe sign-in email address for your SELAR account was changed at " + d.when(d.At) + ". This message was sent to the previous address." +
			"\n\nIf you made this change, you do not need to do anything. If you did not, please contact the SELAR research team." +
			footer(securityFooterReason, d)
	default:
		return "", "", fmt.Errorf("unknown notice kind %q", k)
	}
	return subject, body, nil
}
