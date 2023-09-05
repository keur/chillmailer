package datastore

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

type MailingListInfo struct {
	Name           string
	Description    string
	TimeCreated    time.Time
	NumSubscribers int
}

type SubscriberInfo struct {
	Email      string
	UnsubToken string
	TimeJoined time.Time
}

type Datastore interface {
	InitializeDatabase() error
	GetMailingListID(name string) (int, error)
	CreateMailingList(name string, description string) (int, error)
	SubscribeToMailingList(listID int, email string) error
	UnsubscribeRequest(listID int, email string, unsubToken string) error
	QueryAllMailingLists() ([]MailingListInfo, error)
	QueryMailingListSubscriberInfo(listID int) ([]SubscriberInfo, error)
	RawHandle() *sql.DB
	Close() error
}

type Sqlite struct {
	*sql.DB
}

func NewSqlite(databaseFile string) (*Sqlite, error) {
	db, err := sql.Open("sqlite3", databaseFile)
	if err != nil {
		return nil, err
	}

	sqlite := &Sqlite{db}
	return sqlite, err
}

func (sq *Sqlite) RawHandle() *sql.DB {
	return sq.DB
}

func (sq *Sqlite) Close() error {
	return sq.RawHandle().Close()
}

func (sq *Sqlite) InitializeDatabase() error {
	// Create mailing list table
	sqlStmt := `
    CREATE TABLE IF NOT EXISTS mailing_list (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT UNIQUE,
        description TEXT,
        time_created DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `
	_, err := sq.Exec(sqlStmt)
	if err != nil {
		return err
	}

	// Create subscriptions table
	sqlStmt = `
    CREATE TABLE IF NOT EXISTS subscriptions (
        list_id        INTEGER,
        email          TEXT,
        unsub_token    VARCHAR(36),
        time_joined    DATETIME DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY(list_id) REFERENCES mailing_list(id),
        UNIQUE(list_id, email)
    );
    `
	_, err = sq.Exec(sqlStmt)
	if err != nil {
		return err
	}
	return nil
}

const MailingListNoExist = 0

func (sq *Sqlite) GetMailingListID(name string) (int, error) {
	var listID int
	err := sq.QueryRow("SELECT id FROM mailing_list WHERE name = ?", name).Scan(&listID)
	if err != nil {
		if err == sql.ErrNoRows {
			err = nil
		}
		return MailingListNoExist, err
	}
	return listID, nil
}

func (sq *Sqlite) CreateMailingList(name string, description string) (int, error) {
	listID, err := sq.GetMailingListID(name)
	if err != nil {
		return MailingListNoExist, err
	}

	if listID != MailingListNoExist {
		return listID, nil
	}

	res, err := sq.Exec("INSERT INTO mailing_list (name, description) VALUES (?, ?)", name, description)
	if err != nil {
		return 0, err
	}

	lastInsertID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	return int(lastInsertID), nil
}

func (sq *Sqlite) SubscribeToMailingList(listID int, email string) error {
	unsubToken, err := uuid.NewUUID()
	if err != nil {
		return err
	}
	_, err = sq.Exec("INSERT INTO subscriptions (list_id, email, unsub_token) VALUES (?, ?, ?)", listID, email, unsubToken.String())
	if err != nil {
		return err
	}
	return nil
}

var ErrorBadToken error = errors.New("Bad token provided")

func (sq *Sqlite) UnsubscribeRequest(listID int, email string, unsubToken string) error {
	var actualToken string
	err := sq.QueryRow("SELECT unsub_token FROM subscriptions WHERE list_id = ? AND email = ?", listID, email).Scan(&actualToken)
	if err != nil {
		return err
	}
	if unsubToken != actualToken {
		return ErrorBadToken
	}
	_, err = sq.Exec("DELETE FROM subscriptions WHERE list_id = ? AND email = ?", listID, email)
	if err != nil {
		return err
	}
	return nil
}

func (sq *Sqlite) QueryAllMailingLists() ([]MailingListInfo, error) {
	rows, err := sq.Query(`
      SELECT
          ml.name,
          ml.description,
          ml.time_created,
          COUNT(s.email)
      FROM mailing_list ml
      LEFT JOIN subscriptions s on ml.id = s.list_id
      GROUP BY ml.id;
  `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var infos []MailingListInfo
	for rows.Next() {
		var mlInfo MailingListInfo
		rows.Scan(&mlInfo.Name, &mlInfo.Description, &mlInfo.TimeCreated, &mlInfo.NumSubscribers)
		infos = append(infos, mlInfo)
	}
	return infos, nil
}

func (sq *Sqlite) QueryMailingListSubscriberInfo(listID int) ([]SubscriberInfo, error) {
	rows, err := sq.Query("SELECT email, unsub_token, time_joined FROM subscriptions WHERE list_id = ?", listID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subscribers []SubscriberInfo
	for rows.Next() {
		var sub SubscriberInfo
		rows.Scan(&sub.Email, &sub.UnsubToken, &sub.TimeJoined)
		subscribers = append(subscribers, sub)
	}
	return subscribers, nil
}
