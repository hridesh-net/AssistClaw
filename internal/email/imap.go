package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

type imapBackend struct {
	cfg   *config.Config
	acc   config.EmailAccountConfig
	imap  *config.EmailIMAPConfig
	smtp  *config.EmailSMTPConfig
	store *Store
}

func init() {
	RegisterBackendBuilder("imap", newIMAPBackend)
}

func newIMAPBackend(cfg *config.Config, acc config.EmailAccountConfig, store *Store) (Backend, error) {
	if acc.IMAP == nil || acc.SMTP == nil {
		return nil, fmt.Errorf("imap backend requires imap and smtp blocks")
	}
	return &imapBackend{cfg: cfg, acc: acc, imap: acc.IMAP, smtp: acc.SMTP, store: store}, nil
}

func (b *imapBackend) Name() string { return "imap-" + b.acc.Name }

func (b *imapBackend) dial(ctx context.Context) (*imapclient.Client, error) {
	host := strings.TrimSpace(b.imap.Host)
	useTLS := true
	if b.imap.UseTLS != nil {
		useTLS = *b.imap.UseTLS
	}
	var c *imapclient.Client
	var err error
	if useTLS || strings.Contains(host, ":993") {
		c, err = imapclient.DialTLS(host, &imapclient.Options{
			TLSConfig: &tls.Config{ServerName: hostOnly(host)},
		})
	} else {
		c, err = imapclient.DialStartTLS(host, &imapclient.Options{
			TLSConfig: &tls.Config{ServerName: hostOnly(host)},
		})
	}
	if err != nil {
		return nil, err
	}
	if err := c.Login(b.imap.Username, b.imap.Password).Wait(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func hostOnly(hostport string) string {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return strings.Split(hostport, ":")[0]
	}
	return h
}

func (b *imapBackend) Watch(ctx context.Context, onNew func(context.Context, Ref) error) error {
	iv, err := time.ParseDuration(b.cfg.Email.PollInterval)
	if err != nil {
		iv = 60 * time.Second
	}
	t := time.NewTicker(iv)
	defer t.Stop()
	for {
		_ = b.poll(ctx, onNew)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (b *imapBackend) poll(ctx context.Context, onNew func(context.Context, Ref) error) error {
	c, err := b.dial(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	mbox := strings.TrimSpace(b.imap.Mailbox)
	if mbox == "" {
		mbox = "INBOX"
	}
	sel, err := c.Select(mbox, nil).Wait()
	if err != nil {
		return err
	}
	last, err := b.store.GetLastIMAPUID(b.acc.Name)
	if err != nil {
		return err
	}
	// First sync: advance the UID cursor to the server's UIDNEXT-1 so we only
	// process mail that arrives after AssistClaw is running. Searching only
	// \Unseen misses messages another client (or the host) marks \Seen before we poll.
	if last == 0 && sel != nil && sel.UIDNext > 0 {
		next := uint32(sel.UIDNext)
		if next > 1 {
			baseline := next - 1
			if err := b.store.SetLastIMAPUID(b.acc.Name, baseline); err != nil {
				return err
			}
			last = baseline
		}
	}
	var crit *imap.SearchCriteria
	// Without UIDNEXT we cannot safely use 1:* (would replay the whole mailbox).
	if last == 0 && (sel == nil || sel.UIDNext == 0) {
		crit = &imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}
	} else {
		var uidSet imap.UIDSet
		uidSet.AddRange(imap.UID(last+1), 0) // (last+1):*
		crit = &imap.SearchCriteria{UID: []imap.UIDSet{uidSet}}
	}
	data, err := c.UIDSearch(crit, nil).Wait()
	if err != nil {
		return err
	}
	uids := data.AllUIDs()
	var maxUID uint32
	for _, u := range uids {
		uid := uint32(u)
		if uid <= last {
			continue
		}
		if err := onNew(ctx, Ref{AccountName: b.acc.Name, ProviderID: fmt.Sprintf("%d", uid)}); err != nil {
			return err
		}
		if uid > maxUID {
			maxUID = uid
		}
	}
	if maxUID > last {
		_ = b.store.SetLastIMAPUID(b.acc.Name, maxUID)
	}
	return nil
}

func (b *imapBackend) Fetch(ctx context.Context, ref Ref) (*MailMessage, error) {
	uid := ref.ProviderID
	c, err := b.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	mbox := strings.TrimSpace(b.imap.Mailbox)
	if mbox == "" {
		mbox = "INBOX"
	}
	if _, err := c.Select(mbox, nil).Wait(); err != nil {
		return nil, err
	}
	var u imap.UID
	_, _ = fmt.Sscanf(uid, "%d", &u)
	uidSet := imap.UIDSetNum(u)
	fetchOpts := &imap.FetchOptions{
		Envelope: true,
		UID:      true,
		BodySection: []*imap.FetchItemBodySection{{
			Peek: true,
		}},
	}
	msgs, err := c.Fetch(uidSet, fetchOpts).Collect()
	if err != nil || len(msgs) == 0 {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("no message for uid %s", uid)
	}
	buf := msgs[0]
	env := buf.Envelope
	if env == nil {
		return nil, fmt.Errorf("missing envelope")
	}
	from := formatAddresses(env.From)
	subj := env.Subject
	msgID := env.MessageID
	if msgID != "" && !strings.HasPrefix(msgID, "<") {
		msgID = "<" + msgID + ">"
	}
	body := ""
	if len(buf.BodySection) > 0 && len(buf.BodySection[0].Bytes) > 0 {
		msg, err := mail.ReadMessage(strings.NewReader(string(buf.BodySection[0].Bytes)))
		if err == nil {
			bb, _ := io.ReadAll(msg.Body)
			body = strings.TrimSpace(string(bb))
		} else {
			body = strings.TrimSpace(string(buf.BodySection[0].Bytes))
		}
	}
	headers := map[string]string{}
	if len(buf.BodySection) > 0 && len(buf.BodySection[0].Bytes) > 0 {
		msg, err := mail.ReadMessage(strings.NewReader(string(buf.BodySection[0].Bytes)))
		if err == nil {
			headers["Message-ID"] = msg.Header.Get("Message-ID")
			headers["In-Reply-To"] = msg.Header.Get("In-Reply-To")
			headers["References"] = msg.Header.Get("References")
		}
	}
	mid := headers["Message-ID"]
	if mid == "" {
		mid = msgID
	}
	return &MailMessage{
		Ref:        ref,
		From:       from,
		Subject:    subj,
		BodyText:   body,
		MessageID:  mid,
		InReplyTo:  headers["In-Reply-To"],
		References: headers["References"],
	}, nil
}

func formatAddresses(addrs []imap.Address) string {
	if len(addrs) == 0 {
		return ""
	}
	a := addrs[0]
	s := a.Addr()
	if a.Name != "" {
		return fmt.Sprintf("%s <%s>", a.Name, s)
	}
	return s
}

func (b *imapBackend) Reply(ctx context.Context, m *MailMessage, body string) error {
	_ = ctx
	addr, err := mail.ParseAddress(m.From)
	if err != nil {
		addr = &mail.Address{Address: strings.Trim(m.From, "<>")}
	}
	subj := m.Subject
	if !strings.HasPrefix(strings.ToLower(subj), "re:") {
		subj = "Re: " + subj
	}
	inReplyTo := strings.TrimSpace(m.MessageID)
	ref := strings.TrimSpace(m.References)
	if inReplyTo != "" {
		if ref == "" {
			ref = inReplyTo
		} else {
			ref = ref + " " + inReplyTo
		}
	}
	_, err = b.sendSMTP(addr, subj, body, inReplyTo, ref)
	return err
}

// SendNew composes and sends a brand-new mail (no inbound message required).
// Implements NewMailSender for goal-driven threads.
func (b *imapBackend) SendNew(ctx context.Context, to, subject, body, inReplyTo, references string) (string, error) {
	_ = ctx
	addr, err := mail.ParseAddress(to)
	if err != nil {
		addr = &mail.Address{Address: strings.Trim(to, "<>")}
	}
	return b.sendSMTP(addr, subject, body, strings.TrimSpace(inReplyTo), strings.TrimSpace(references))
}

// sendSMTP is the shared compose+send path. It always stamps a generated
// Message-ID so future replies can be threaded back to us, and returns it.
func (b *imapBackend) sendSMTP(to *mail.Address, subject, body, inReplyTo, references string) (string, error) {
	port := b.smtp.Port
	if port == 0 {
		port = 587
	}
	host := strings.TrimSpace(b.smtp.Host)
	user := b.smtp.Username
	pass := b.smtp.Password
	from := user
	msgID, err := generateMessageID(hostOnly(host))
	if err != nil {
		return "", err
	}
	var hdr strings.Builder
	fmt.Fprintf(&hdr, "From: %s\r\n", from)
	fmt.Fprintf(&hdr, "To: %s\r\n", to.String())
	fmt.Fprintf(&hdr, "Subject: %s\r\n", subject)
	fmt.Fprintf(&hdr, "Message-ID: %s\r\n", msgID)
	if inReplyTo != "" {
		fmt.Fprintf(&hdr, "In-Reply-To: %s\r\n", inReplyTo)
	}
	if references != "" {
		fmt.Fprintf(&hdr, "References: %s\r\n", references)
	}
	fmt.Fprintf(&hdr, "MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", body)
	addrStr := host
	if !strings.Contains(addrStr, ":") {
		addrStr = fmt.Sprintf("%s:%d", host, port)
	}
	auth := smtp.PlainAuth("", user, pass, hostOnly(host))
	if err := smtp.SendMail(addrStr, auth, from, []string{to.Address}, []byte(hdr.String())); err != nil {
		return "", err
	}
	return msgID, nil
}

// generateMessageID builds an RFC 5322 Message-ID under our control.
func generateMessageID(domain string) (string, error) {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	if domain == "" {
		domain = "assistclaw.local"
	}
	return fmt.Sprintf("<ac-%d-%s@%s>", time.Now().Unix(), hex.EncodeToString(b[:]), domain), nil
}
