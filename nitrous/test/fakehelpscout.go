package test

import "github.com/nitrous-io/dedicated/nitrous/pkg/helpscout"

type FakeHelpScout struct {
	CreateConversationCalled int
	Customer                 helpscout.Person
	Subject                  string
	Mailbox                  helpscout.MailboxRef
	Threads                  []*helpscout.Thread
	Error                    HelpScoutError
}

type HelpScoutError struct {
	Code  int
	Error string
}

func (c *FakeHelpScout) CreateConversation(customer helpscout.Person, subject string, mailbox helpscout.MailboxRef, threads []*helpscout.Thread) error {
	c.CreateConversationCalled += 1
	c.Customer = customer
	c.Subject = subject
	c.Mailbox = mailbox
	c.Threads = threads

	// Fix err return
	return nil
}

//var InvalidJSON = HelpScoutError{
//	Code:  400,
//	Error: "Invalid Json",
//}
//
//var InvalidAuth = HelpScoutError{
//	Code:  401,
//	Error: "Authentication was not provided or was invalid.",
//}
//
//var InvalidHeader = HelpScoutError{
//	Code:  400,
//	Error: "Request requires header: 'Content-Type: application/json'",
//}
//
//// TODO: Actually send valid request to get example JSON response.
//var HelpScoutCreateConversationResponse = `{
//		"id": 123,
//		"email": "example.com",
//		"threads": {
//			"customer": 1,
//				}
//		},
//	}`
