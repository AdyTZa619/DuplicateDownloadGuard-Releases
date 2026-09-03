package main

import (
	"context"
	"errors"
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
	case containsAny(text, "eoverquota", "over quota", "overquota", "transfer quota", "bandwidth quota", "quota exceeded", "exceeded your transfer"):
		return problem("MEGA_QUOTA", "Cotă MEGA depășită", "MEGA a refuzat transferul deoarece limita de transfer a fost atinsă.", "Jobul a fost pus pe pauză. Reia-l după ce MEGA permite din nou transferul.", false)
	case containsAny(text, "eblocked", "account blocked", "account has been suspended", "link has been blocked", "copyright violation"):
		return problem("MEGA_BLOCKED", "Acces MEGA blocat", "Contul sau linkul a fost blocat de MEGA și nu poate fi folosit.", "Verifică linkul în browser sau folosește o sursă validă. Reîncercarea automată nu ajută.", false)
	case containsAny(text, "not logged in", "not logged", "not logged-in", "please log in", "please login"):
		return problem("MEGA_AUTH", "Sesiune MEGA indisponibilă", "MEGAcmd nu are o sesiune valabilă pentru această operație.", "Scanează din nou linkul MEGA, apoi reia jobul.", false)
	case containsAny(text, "invalid key", "decryption key", "missing key", "ekey", "api_ekey"):
		return problem("MEGA_KEY", "Cheie MEGA invalidă", "Linkul nu conține o cheie de decriptare validă sau cheia nu corespunde.", "Corectează linkul MEGA și scanează din nou.", false)
	case containsAny(text, "invalid link", "malformed link", "not a valid mega", "bad arguments", "eargs"):
		return problem("MEGA_LINK", "Link MEGA invalid", "MEGAcmd nu recunoaște linkul primit ca folder sau fișier public valid.", "Copiază din nou linkul complet din MEGA, inclusiv cheia de după #.", false)
	case containsAny(text, "enoent", "api_enoent", "not found", "does not exist", "could not find"):
		return problem("MEGA_NOT_FOUND", "Fișier MEGA negăsit", "Fișierul sau folderul nu mai există la calea scanată.", "Scanează din nou folderul pentru a actualiza lista.", false)
	case containsAny(text, "enospc", "no space left", "disk full", "not enough space"):
		return problem("DISK_FULL", "Spațiu insuficient", "Windows sau MEGAcmd nu poate scrie fișierul deoarece discul nu are spațiu disponibil.", "Eliberează spațiu în folderul de download, apoi reia jobul.", false)
	case containsAny(text, "eaccess", "api_eaccess", "access denied", "permission denied", "not permitted"):
		return problem("ACCESS_DENIED", "Acces refuzat", "Fișierul nu poate fi scris sau citit cu permisiunile actuale.", "Verifică folderul de download și protecția antivirus, apoi reia jobul.", false)
	case containsAny(text, "etoomany", "api_etoomany", "too many requests", "rate limit", "too many connections"):
		return problem("MEGA_RATE_LIMIT", "Prea multe cereri MEGA", "MEGA limitează temporar numărul de cereri sau conexiuni.", "Programul va reîncerca; dacă persistă, pune coada pe pauză câteva minute.", true)
	case containsAny(text, "etempunavail", "api_etempunavail", "temporarily unavailable", "temporary unavailable", "try again later"):
		return problem("MEGA_TEMPORARY", "MEGA temporar indisponibil", "Serverul sau fișierul nu este disponibil momentan.", "Programul va reîncerca automat.", true)
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
