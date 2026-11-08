package tata

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
)

// MailConfig 对应 config.yaml 的 mail。SMTP 发邮箱验证码；163 邮箱填 smtp.163.com:465（SSL）。
type MailConfig struct {
	Enabled  bool
	Host     string
	Port     int
	Username string
	Password string
	From     string
	FromName string
}

type Mailer interface {
	Ready() bool
	SendCode(email, code, scene string) error
}

type SMTPMailer struct {
	cfg  MailConfig
	send func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

func NewSMTPMailer(cfg MailConfig) *SMTPMailer {
	return &SMTPMailer{cfg: cfg}
}

func (m *SMTPMailer) Ready() bool {
	if m == nil || !m.cfg.Enabled {
		return false
	}
	return strings.TrimSpace(m.cfg.Host) != "" && strings.TrimSpace(m.fromAddr()) != ""
}

func (m *SMTPMailer) fromAddr() string {
	if m == nil {
		return ""
	}
	if from := strings.TrimSpace(m.cfg.From); from != "" {
		return from
	}
	return strings.TrimSpace(m.cfg.Username)
}

func (m *SMTPMailer) fromName() string {
	if m == nil {
		return "塔塔"
	}
	if name := strings.TrimSpace(m.cfg.FromName); name != "" {
		return name
	}
	return "塔塔"
}

func (m *SMTPMailer) port() int {
	if m == nil || m.cfg.Port <= 0 {
		return 465
	}
	return m.cfg.Port
}

func emailCodeCopy(scene, code string) (subject, body string) {
	purpose := "完成身份验证"
	switch scene {
	case "register":
		purpose = "注册账号"
	case "bind":
		purpose = "绑定邮箱"
	}
	subject = "【塔塔】邮箱验证码"
	body = "您好：\r\n\r\n" +
		"我们收到了使用此邮箱在塔塔" + purpose + "的请求。请使用以下验证码完成验证：\r\n\r\n" +
		code + "\r\n\r\n" +
		"该验证码 5 分钟内有效，请勿转发或告知他人。若非您本人操作，可忽略本邮件，账号不会因此发生变化。\r\n\r\n" +
		"塔塔\r\n"
	return subject, body
}

func (m *SMTPMailer) SendCode(email, code, scene string) error {
	if !m.Ready() {
		return biz("邮件服务未配置")
	}
	from := m.fromAddr()
	headerFrom := mime.QEncoding.Encode("UTF-8", m.fromName()) + " <" + from + ">"
	subject, body := emailCodeCopy(scene, code)
	msg := []byte("From: " + headerFrom + "\r\n" +
		"To: " + email + "\r\n" +
		"Subject: " + mime.QEncoding.Encode("UTF-8", subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" + body)
	addr := net.JoinHostPort(strings.TrimSpace(m.cfg.Host), fmt.Sprintf("%d", m.port()))
	var auth smtp.Auth
	if user := strings.TrimSpace(m.cfg.Username); user != "" {
		auth = smtp.PlainAuth("", user, m.cfg.Password, strings.TrimSpace(m.cfg.Host))
	}
	send := m.send
	if send == nil {
		send = smtpSend
	}
	if err := send(addr, auth, from, []string{email}, msg); err != nil {
		return fmt.Errorf("send mail: %w", err)
	}
	return nil
}

func smtpSend(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if port == "465" {
		return smtpSendTLS(addr, host, a, from, to, msg)
	}
	return smtp.SendMail(addr, a, from, to, msg)
}

func smtpSendTLS(addr, host string, a smtp.Auth, from string, to []string, msg []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return err
	}
	defer conn.Close()
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer c.Close()
	if a != nil {
		if err := c.Auth(a); err != nil {
			return err
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
