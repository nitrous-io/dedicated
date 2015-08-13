package test

import (
	"github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"
)

type FakeHelpScout struct {
	CreateConversationCalled int
	Customer                 helpscout.Person
	Subject                  string
	Mailbox                  helpscout.MailboxRef
	Threads                  []*helpscout.Thread
}

func (c *FakeHelpScout) CreateConversation(customer helpscout.Person, subject string, mailbox helpscout.MailboxRef, threads []*helpscout.Thread) error {
	c.CreateConversationCalled += 1
	c.Customer = customer
	c.Subject = subject
	c.Mailbox = mailbox
	c.Threads = threads

	return nil
}
