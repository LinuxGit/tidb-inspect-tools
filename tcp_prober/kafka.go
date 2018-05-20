package main

import (
	"encoding/json"
	"net/smtp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/ngaut/log"
)

const (
	timeFormat    = "2006-01-02 15:04:05"
	maxRetry      = 12
	retryInterval = 5 * time.Second
)

//KafkaMsg represents kafka message
type KafkaMsg struct {
	Title       string `json:"event_object"`
	Source      string `json:"object_name"`
	Instance    string `json:"object_ip"`
	Description string `json:"event_msg"`
	Time        string `json:"event_time"`
	Level       string `json:"event_level"`
	Summary     string `json:"summary"`
	Expr        string `json:"expr"`
	Value       string `json:"value"`
	URL         string `json:"url"`
}

//Run represents runtime information
type Run struct {
}

//PushKafkaMsg sends email to smtp server
func (r *Run) PushKafkaMsg(alertname, msg string) error {
	// Set up authentication information.
	auth := smtp.PlainAuth(
		"",
		*SMTPAuthUsername,
		*SMTPAuthPassword,
		*SMTPSmarthost,
	)
	to := strings.Split(*SMTPTo, ",")
	user := *SMTPFrom
	subject := alertname
	content_type := "Content-Type: text/plain; charset=UTF-8"
	body := msg
	message := []byte("To: " + *SMTPTo + "\r\nFrom: " + user +
		"<" + user + ">\r\nSubject: " + subject + "\r\n" + content_type + "\r\n\r\n" + body)

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step
	err := smtp.SendMail(*SMTPSmarthost+":25", auth, user, to, message)
	if err != nil {
		return errors.Trace(err)
	}

	log.Infof("send email %s:%s to smtp server", alertname, message)
	return nil
}

//TransferData transfers AlertData to string and sends message to kafka
func (r *Run) TransferData(alertname, env, instance, level, summary string) {
	kafkaMsg := &KafkaMsg{
		Title:       alertname,
		Source:      env,
		Instance:    instance,
		Description: summary,
		Time:        time.Now().Format(timeFormat),
		Level:       level,
		Summary:     summary,
		Expr:        "",
		Value:       "",
		URL:         "",
	}

	alertByte, err := json.Marshal(kafkaMsg)
	if err != nil {
		log.Errorf("Failed to marshal KafkaMsg: %v", err)
		return
	}

	for i := 0; i < maxRetry; i++ {
		if err := r.PushKafkaMsg(alertname, string(alertByte)); err != nil {
			log.Errorf("Failed to send email to smtp server: %v", err)
			time.Sleep(retryInterval)
			continue
		}
		return
	}
}

//Scheduler probes services tcp port
func (r *Run) Scheduler() {
	log.Infof("tcp_prober config: %+v", probeConfig)
	for {
		for _, attr := range probeConfig.Service {
			aliveStatus := probeTCP(attr.Addr)
			if !aliveStatus {
				log.Errorf("Failed to dial %s, alert summary is %s", attr.Addr, attr.Summary)
				r.TransferData(attr.Alertname, *clusterName, attr.Addr, attr.Level, attr.Summary)
			}
		}

		time.Sleep(time.Second * 60)
	}
}
