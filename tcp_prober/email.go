package main

import (
	"bytes"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/juju/errors"
	"github.com/ngaut/log"
)

type loginAuth struct {
	username, password string
}

// LoginAuth ... implements stmp.Auth
func LoginAuth(username, password string) smtp.Auth {
	return &loginAuth{username, password}
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", []byte{}, nil
}

// Used for AUTH LOGIN. (Maybe password should be encrypted)
func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		switch strings.ToLower(string(fromServer)) {
		case "username:":
			return []byte(a.username), nil
		case "password:":
			return []byte(a.password), nil
		default:
			return nil, errors.New("unexpected server challenge")
		}
	}
	return nil, nil
}

// PushKafkaMsg ... sends alert messages to SMTP server
func (r *Run) PushKafkaMsg(alertname, alertmsg string) error {
	body := alertmsg
	from := *smtpFrom
	to := *smtpTo

	// Setup headers
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = fmt.Sprintf("[FIRING] %s %s", alertname, *clusterName)

	// Setup message
	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + body

	var c *smtp.Client
	c, err := smtp.Dial(*smtpSmarthost)
	if err != nil {
		return errors.Errorf("dial smtp server %s", err)
	}

	if ok, _ := c.Extension("AUTH"); ok {
		auth := LoginAuth(*smtpAuthUsername, *smtpAuthPassword)

		if auth != nil {
			if err = c.Auth(auth); err != nil {
				return errors.Errorf("%T failed: %s", auth, err)
			}
		}
	}

	defer c.Quit()

	if err = c.Mail(from); err != nil {
		return errors.Errorf("sending mail from: %s", err)
	}

	addrs, err := mail.ParseAddressList(to)
	if err != nil {
		return errors.Errorf("parsing to addresses: %s", err)
	}
	for _, addr := range addrs {
		if err = c.Rcpt(addr.Address); err != nil {
			return errors.Errorf("sending rcpt to: %s", err)
		}
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return errors.Trace(err)
	}
	defer wc.Close()

	buf := bytes.NewBufferString(message)
	if _, err = buf.WriteTo(wc); err != nil {
		return errors.Trace(err)
	}

	log.Infof("send email %s:%s to smtp server", alertname, message)
	return nil
}
