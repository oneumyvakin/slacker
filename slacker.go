package slacker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	NotifyAlways   int = 0
	NotifyOnceHour int = 1
	NotifyOnceDay  int = 2

	DefaultDatabaseFilePath string = "slacker.json"
	DefaultMessageTag       string = "default_tag"
	DefaultUsername         string = "Slacker Notifier"
	DefaultIconEmoji        string = ":ghost:"
)

// Slacker sends notification tagged by MessageTag with Frequency
type Slacker struct {
	Hook             string
	Log              *log.Logger
	IconEmoji        string
	From             string
	To               []Recipient // Required
	Frequency        int
	MessageTag       string
	DatabaseFilePath string
	httpClient       *http.Client
}

type SlackMessage struct {
	Channel   string `json:"channel"`
	Username  string `json:"username"`
	Text      string `json:"text"`
	IconEmoji string `json:"icon_emoji"`
}

// Recipient holds Channel and Username
type Recipient struct {
	Channel  string
	Username string
}

// Send message with subject
func (slacker Slacker) Send(message string) error {
	if err := slacker.setDefaults(); err != nil {
		return fmt.Errorf("Slacker failed to send message: %s", err)
	}

	hash := slacker.getHash(message)
	if !slacker.needToSend(hash) {
		slacker.Log.Printf("Skip message %s: %s", hash, message)
		return nil
	}

	slackMessage := SlackMessage{
		IconEmoji: slacker.IconEmoji,
		Username:  slacker.From,
	}

	for _, recipient := range slacker.To {
		slackMessage.Channel = recipient.Channel
		slackMessage.Text = recipient.Username + " " + message

		response, err := slacker.send(slackMessage)
		if err != nil {
			slacker.Log.Printf("Slacker failed to send message: %s", err)
			return err
		}

		slacker.Log.Printf("Send message %s: %s %s", slacker.MessageTag, message, response)
	}

	err := slacker.addToDb(hash, message)
	if err != nil {
		slacker.Log.Printf("Slacker failed to send message: %s", err)
		return err
	}

	return nil
}

func (slacker *Slacker) send(message SlackMessage) (response string, err error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return "", err
	}

	if slacker.httpClient == nil {
		slacker.setHttpClient()
	}

	raw_response, err := slacker.httpClient.Post(slacker.Hook, "string", bytes.NewBuffer(payload))
	if err != nil {
		return "", err
	}

	byte_response, err := ioutil.ReadAll(raw_response.Body)
	if err != nil {
		return "", err
	}

	response = string(byte_response)
	if response != "ok" {
		return "", fmt.Errorf("Response from Slack: %s", response)
	}

	return
}

func (slacker *Slacker) setDefaults() error {
	if slacker.Hook == "" {
		return errors.New("Web hook url is not set")
	}

	if len(slacker.To) == 0 {
		return errors.New("Recipients are not set")
	}

	if slacker.IconEmoji == "" {
		slacker.IconEmoji = DefaultIconEmoji
	}

	if slacker.From == "" {
		slacker.From = DefaultUsername
	}

	if slacker.Log == nil {
		slacker.Log = log.New(os.Stdout, DefaultUsername+" ", log.LstdFlags)
	}

	if slacker.MessageTag == "" {
		slacker.MessageTag = DefaultMessageTag
	}

	if slacker.DatabaseFilePath == "" {
		slacker.DatabaseFilePath = DefaultDatabaseFilePath
	}

	return nil
}

func (slacker Slacker) needToSend(hash string) bool {
	if slacker.Frequency == NotifyAlways {
		return true
	}

	if slacker.inDb(hash) {
		return false
	}

	return true
}

func (slacker Slacker) getHash(subject string) (hash string) {
	t := time.Now()

	if slacker.Frequency == NotifyOnceHour {
		hash = t.Format("2006-01-02-15") + ":" + slacker.MessageTag + ":" + subject
		return
	}

	if slacker.Frequency == NotifyOnceDay {
		hash = t.Format("2006-01-02") + ":" + slacker.MessageTag + ":" + subject
		return
	}

	return
}

func (slacker Slacker) addToDb(hash string, subject string) error {
	db, err := slacker.loadDb()
	if err != nil {
		return fmt.Errorf("Slacker failed to add %s:%s to database: %s", hash, subject, err)
	}

	db[hash] = subject

	err = slacker.saveDb(db)
	if err != nil {
		return fmt.Errorf("Slacker failed to add %s to database: %s", hash, err)
	}

	return nil
}

func (slacker Slacker) inDb(hash string) bool {
	db, err := slacker.loadDb()
	if err != nil {
		slacker.Log.Printf("Slacker failed to load database: %s", err)
		return false
	}

	if _, ok := db[hash]; ok == true {
		return true
	}

	return false
}

func (slacker Slacker) saveDb(db map[string]string) (err error) {
	dbFile, err := os.OpenFile(slacker.DatabaseFilePath, os.O_WRONLY, os.ModeExclusive)
	if err != nil {
		return fmt.Errorf("Slacker failed to save database: Failed to open database file: %s", err)
	}
	defer dbFile.Close()

	err = json.NewEncoder(dbFile).Encode(db)
	if err != nil {
		return fmt.Errorf("Slacker failed to save database: Failed to encode json to file: %s", err)
	}

	return
}

func (slacker Slacker) loadDb() (db map[string]string, err error) {
	dbFile, err := os.Open(slacker.DatabaseFilePath)
	if err != nil {
		dbFile, err = slacker.createDb()
		if err != nil {
			return nil, fmt.Errorf("Slacker failed to load database: %s", err)
		}
	}
	defer dbFile.Close()

	db = make(map[string]string)
	err = json.NewDecoder(dbFile).Decode(&db)
	if err != nil {
		return nil, fmt.Errorf("Slacker failed to load database: %s", err)
	}

	return
}

func (slacker Slacker) createDb() (dbFile *os.File, err error) {
	dbFile, err = os.Create(slacker.DatabaseFilePath)
	if err != nil {
		return nil, fmt.Errorf("Slacker failed to create database file %s: %s", slacker.DatabaseFilePath, err)
	}
	err = dbFile.Truncate(0)
	if err != nil {
		return nil, fmt.Errorf("Slacker failed to create database file %s: %s", slacker.DatabaseFilePath, err)
	}
	_, err = dbFile.Write([]byte("{}"))
	if err != nil {
		return nil, fmt.Errorf("Slacker failed to create database file %s: %s", slacker.DatabaseFilePath, err)
	}
	return
}

func (slacker *Slacker) setHttpClient() {
	tr := &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: time.Second * 10,
		MaxIdleConnsPerHost:   128,
	}
	slacker.httpClient = &http.Client{Transport: tr}
}
