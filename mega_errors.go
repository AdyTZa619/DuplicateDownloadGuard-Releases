package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type MegaProblem struct {
	Code      string `json:"code"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Action    string `json:"action"`
	Retryable bool   `json:"retryable"`
}

type MegaProblemError struct {
	Problem MegaProblem
	Raw     string
}

func (e *MegaProblemError) Error() string {
	return megaProblemText(e.Problem)
}

func newMegaProblemError(problem MegaProblem, raw string) error {
	return &MegaProblemError{Problem: problem, Raw: sanitizeMega(raw)}
}

func megaProblemFromError(err error) MegaProblem {
	var typed *MegaProblemError
	if errors.As(err, &typed) {
		return typed.Problem
	}
	return classifyMegaProblem("", err)
}

// Keep the cue and the numeric value separate. The older expression allowed the
// generic "quota..." prefix to consume the first digit of a value (12 -> 2),
// producing incorrect retry times. These forms intentionally recognize only a
// duration explicitly tied to retry/wait/reset wording.
var megaRetryDurationRxV85 = regexp.MustCompile(`(?i)(?:retry(?:\s+(?:after|in))?|try\s+again(?:\s+(?:after|in))?|wait(?:\s+for)?|available\s+again(?:\s+(?:after|in))?|reset(?:s)?(?:\s+(?:after|in))?|quota(?:\s+reset)?(?:\s+(?:after|in)))\D{0,20}([0-9]{1,6})\s*(hours?|hrs?|h|minutes?|mins?|m|seconds?|secs?|s)\b`)
var megaRetryClockRxV85 = regexp.MustCompile(`(?i)(?:retry(?:\s+(?:after|in))?|try\s+again(?:\s+(?:after|in))?|wait(?:\s+for)?|available\s+again(?:\s+(?:after|in))?|reset(?:s)?(?:\s+(?:after|in))?|quota(?:\s+reset)?(?:\s+(?:after|in)))\D{0,20}([0-9]{1,2}):([0-9]{2})(?::([0-9]{2}))?`)

func megaRetrySecondsV85(raw string) int64 {
	if m := megaRetryClockRxV85.FindStringSubmatch(raw); len(m) >= 3 {
		a, _ := strconv.ParseInt(m[1], 10, 64)
		b, _ := strconv.ParseInt(m[2], 10, 64)
		if len(m) >= 4 && m[3] != "" {
			c, _ := strconv.ParseInt(m[3], 10, 64)
			// HH:MM:SS when three fields are present.
			return a*3600 + b*60 + c
		}
		// MM:SS for two fields.
		return a*60 + b
	}
	m := megaRetryDurationRxV85.FindStringSubmatch(raw)
	if len(m) < 3 {
		return 0
	}
	n, _ := strconv.ParseInt(m[1], 10, 64)
	unit := strings.ToLower(m[2])
	switch {
	case strings.HasPrefix(unit, "h"):
		return n * 3600
	case strings.HasPrefix(unit, "m"):
		return n * 60
	default:
		return n
	}
}

func megaRetryHumanV85(seconds int64) string {
	if seconds <= 0 {
		return ""
	}
	if seconds < 60 {
		return fmt.Sprintf("~%d secunde", seconds)
	}
	if seconds < 3600 {
		mins := (seconds + 59) / 60
		return fmt.Sprintf("~%d minute", mins)
	}
	hours := seconds / 3600
	mins := (seconds % 3600) / 60
	if mins == 0 {
		return fmt.Sprintf("~%d ore", hours)
	}
	return fmt.Sprintf("~%dh %dm", hours, mins)
}

func megaActionWithRetryV85(action, raw string) string {
	if seconds := megaRetrySecondsV85(raw); seconds > 0 {
		return strings.TrimSpace(action) + " Timp indicat de MEGA: " + megaRetryHumanV85(seconds) + "."
	}
	return action
}

func classifyMegaProblem(output string, err error) MegaProblem {
	raw := strings.TrimSpace(output)
	if err != nil {
		if raw != "" {
			raw += " "
		}
		raw += err.Error()
	}
	text := strings.ToLower(raw)
	problem := func(code, title, message, action string, retryable bool) MegaProblem {
		return MegaProblem{Code: code, Title: title, Message: message, Action: action, Retryable: retryable}
	}
	switch {
	case errors.Is(err, context.Canceled):
		return problem("CANCELLED", "Operație MEGA anulată", "Operația a fost oprită la cererea utilizatorului sau la închiderea aplicației.", "Poți relua operația când dorești.", false)
	case containsAny(text, "checksum", "dimensiune diferită după download", "download incomplet", "rezultatul downloadului este un folder"):
		return problem("DOWNLOAD_VERIFY_FAILED", "Fișier MEGA invalid după download", "Fișierul rezultat nu corespunde dimensiunii sau checksum-ului cunoscut pentru sursa remote.", "Fișierul nu este marcat finalizat. Verifică discul și sursa înainte de o nouă încercare.", false)
	case containsAny(text, "api_eoverquota", "eoverquota", "over quota", "overquota", "transfer quota", "bandwidth quota", "quota exceeded", "exceeded your transfer", "bandwidth limit exceeded", "http 509", "status 509"):
		return problem("MEGA_QUOTA", "Cotă MEGA depășită", "MEGA a refuzat transferul deoarece limita de transfer a fost atinsă.", megaActionWithRetryV85("Jobul a fost pus pe pauză. Reia-l după ce MEGA permite din nou transferul.", raw), false)
	case containsAny(text, "eblocked", "account blocked", "account has been suspended", "link has been blocked", "copyright violation", "api_ebusinesspastdue"):
		return problem("MEGA_BLOCKED", "Acces MEGA blocat", "Contul sau linkul a fost blocat ori restricționat de MEGA și nu poate fi folosit.", "Verifică linkul/contul în browser sau folosește o sursă validă. Reîncercarea automată nu ajută.", false)
	case containsAny(text, "not logged in", "not logged", "not logged-in", "please log in", "please login", "http 401", "status 401"):
		return problem("MEGA_AUTH", "Sesiune MEGA indisponibilă", "MEGAcmd nu are o sesiune valabilă pentru această operație.", "Scanează din nou linkul MEGA, apoi reia jobul.", false)
	case containsAny(text, "invalid key", "decryption key", "missing key", "ekey", "api_ekey"):
		return problem("MEGA_KEY", "Cheie MEGA invalidă", "Linkul nu conține o cheie de decriptare validă sau cheia nu corespunde.", "Corectează linkul MEGA și scanează din nou.", false)
	case containsAny(text, "invalid link", "malformed link", "not a valid mega", "bad arguments", "eargs"):
		return problem("MEGA_LINK", "Link MEGA invalid", "MEGAcmd nu recunoaște linkul primit ca folder sau fișier public valid.", "Copiază din nou linkul complet din MEGA, inclusiv cheia de după #.", false)
	case containsAny(text, "enoent", "api_enoent", "not found", "does not exist", "could not find", "http 404", "status 404"):
		return problem("MEGA_NOT_FOUND", "Fișier MEGA negăsit", "Fișierul sau folderul nu mai există la calea scanată.", "Scanează din nou folderul pentru a actualiza lista.", false)
	case containsAny(text, "enospc", "no space left", "disk full", "not enough space"):
		return problem("DISK_FULL", "Spațiu insuficient", "Windows sau MEGAcmd nu poate scrie fișierul deoarece discul nu are spațiu disponibil.", "Eliberează spațiu în folderul de download, apoi reia jobul.", false)
	case containsAny(text, "eaccess", "api_eaccess", "access denied", "permission denied", "not permitted", "http 403", "status 403"):
		return problem("ACCESS_DENIED", "Acces refuzat", "Fișierul nu poate fi scris sau citit cu permisiunile actuale.", "Verifică folderul de download și protecția antivirus, apoi reia jobul.", false)
	case containsAny(text, "etoomany", "api_etoomany", "too many requests", "rate limit", "too many connections", "http 429", "status 429"):
		return problem("MEGA_RATE_LIMIT", "Prea multe cereri MEGA", "MEGA limitează temporar numărul de cereri sau conexiuni.", megaActionWithRetryV85("Programul va reîncerca; dacă persistă, pune coada pe pauză câteva minute.", raw), true)
	case containsAny(text, "etempunavail", "api_etempunavail", "temporarily unavailable", "temporary unavailable", "try again later"):
		return problem("MEGA_TEMPORARY", "MEGA temporar indisponibil", "Serverul sau fișierul nu este disponibil momentan.", megaActionWithRetryV85("Programul va reîncerca automat.", raw), true)
	case errors.Is(err, context.DeadlineExceeded) || containsAny(text, "timeout", "timed out"):
		return problem("MEGA_TIMEOUT", "MEGA nu a răspuns la timp", "Operația MEGAcmd a depășit timpul maxim de așteptare.", "Programul va reîncerca automat.", true)
	case containsAny(text, "network", "connection", "couldn't connect", "could not connect", "eagain", "api request failed"):
		return problem("MEGA_NETWORK", "Problemă de rețea MEGA", "Conexiunea către MEGA a fost întreruptă sau serverul nu a răspuns.", "Programul va reîncerca automat.", true)
	default:
		message := "MEGAcmd a returnat o eroare necunoscută."
		if err == nil && raw == "" {
			message = "MEGAcmd nu a returnat nici rezultat, nici explicație."
		}
		return problem("MEGA_UNKNOWN", "Eroare MEGA", message, "Deschide Jurnalul tehnic pentru răspunsul brut și reîncearcă o singură dată.", true)
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func megaProblemText(problem MegaProblem) string {
	return problem.Title + ": " + problem.Message + " " + problem.Action
}
