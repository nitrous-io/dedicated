/*
cfinder is an cops interface to finder.

This supports file-find and search in a directory.

File-find returns a list of file paths that match the given term. Its message
format is:

--> h|{ "namespace":"finder", "event: "file_find", "cid": 42, "args": {"term":"asd", "root":"/home/action"} }
<-- h|{ "namespace":"finder", "event: "file_find", "cid": 42, "args": {"term":"asd", "root":"/home/action"} } # signal that we received the message
<-- h|{ "namespace":"finder", "event: "file_find.output", "args": {"term":"asd", "root":"/home/action", "paths": ["workspace/asd"]} }
<-- h|{ "namespace":"finder", "event: "file_find.error", "args": {"term":"asd", "root":"/home/action", "error": "some error"}} # error occurs in the middle of file-find op
<-- h|{ "namespace":"finder", "event: "file_find.completed", "args": {"term":"asd", "root":"/home/action"}} # file find completed

--> h|{ "namespace":"finder", "event: "file_find", "cid": 43, "args": {"term":"another", "root":"/home/action"} } # another request
<-- h|{ "namespace":"finder", "event: "file_find", "cid": 43, "error": "failed to find file" } # if an error occurs before the file-find op starts

Search returns a list of files that contain the given term. Each file includes
metadata that indicates where the given term was matched (i.e. line number and
surrounding content). Its message format is:

--> h|{ "namespace":"finder", "event: "search", "cid": 44, "args": ["123", "asd", "/home/action"] } # args is [id, term, root]
<-- h|{ "namespace":"finder", "event: "search", "cid": 44, "args": ["123", "asd", "/home/action"] }
<-- h|{ "namespace":"finder", "event: "search.output", "args": {"id": "123", "term":"asd", "root":"/home/action", "results": {"file":"filename", "matches": [{"row": 12, "content": "line asd"]}} }
<-- h|{ "namespace":"finder", "event: "search.error", "args": {"id": "123", "term":"asd", "root":"/home/action", "error": "some error"} } # error occurs in the middle of search op
<-- h|{ "namespace":"finder", "event: "search.completed", "args": {"id": "123", "term":"asd", "root":"/home/action"} } # search completed
--> h|{ "namespace":"finder", "event: "search", "cid": 45, "args": ["123", "another", "/home/action"] }
<-- h|{ "namespace":"finder", "event: "search", "cid": 45, "error": "filesystem disappeared" } # if an error occurs before the search op starts
*/
package cfinder

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/nitrous-io/dedicated/freighter/cops"
	"github.com/nitrous-io/dedicated/pkg/finder"
	"github.com/nitrous-io/dedicated/pkg/rootfs"
)

var (
	RootPrefix         = "/var/lib/docker/aufs/mnt"
	chanSize           = 10  // default channel size
	fileFindLimit      = 10  // max number of response for file_find
	contentSearchLimit = 500 // max number of response for search
)

type FileFindResponse struct {
	Term  string   `json:"term"`
	Root  string   `json:"root"`
	Error error    `json:"error"`
	Paths []string `json:"paths"`
}

type SearchResponse struct {
	ID      string                 `json:"id"`
	Term    string                 `json:"term"`
	Root    string                 `json:"root"`
	Error   string                 `json:"error"`
	Results []*finder.SearchResult `json:"results"`
}

// Finder implements AsyncHandler and not Handler because we want to return
// response to the user asynchronously. The flow of response is
//
// --> h|{"event": "file_find", "cid": 1, ...}
// <-- h|{"event": "file_find", "cid": 1, ...} // this signal that we are processing it
// <-- h|{"event": "file_find.output", args: [{"paths": ["container.go"], ..}], ...}
//
// the client should catch `{{event}}.output` content and pring output
// accordingly
type Finder struct {
	id string
	fs *rootfs.Rootfs
}

// Handler interface is not supported by Finder. Caller should upgrade it to
// AsyncHandler interface
func (c *Finder) Handle(msg *cops.Message) *cops.Message {
	return makeErr(msg, fmt.Errorf("invalid message %s", msg.Event))
}

// HandleAsync returns the responses as a channel of messages
func (c *Finder) HandleAsync(msg *cops.Message) chan *cops.Message {
	resp := make(chan *cops.Message, chanSize)
	switch msg.Event {
	case "file_find":
		term := msg.Args[0].(string)
		root := msg.Args[1].(string)
		ch, errch, err := c.fileFind(term, root)
		if err != nil {
			resp <- makeErr(msg, err)
			close(resp)
			return resp
		}
		go func() {
			for err := range errch {
				resp <- makeMessage("file_find.error", FileFindResponse{
					Term:  term,
					Root:  root,
					Error: err,
				})
			}
		}()
		go func() {
			for r := range ch {
				resp <- makeMessage("file_find.output", FileFindResponse{
					Term:  term,
					Root:  root,
					Paths: []string{r},
				})
			}
			resp <- makeMessage("file_find.completed", FileFindResponse{
				Term: term,
				Root: root,
			})
			close(resp)
		}()
		// reply the original message to signal that we are processsing it
		resp <- msg
		return resp
	case "search":
		id, _ := msg.Args[0].(string) // It's ok if we don't get an id.
		term, ok := msg.Args[1].(string)
		if !ok {
			resp <- makeErr(msg, errors.New("term argument is required"))
			close(resp)
			return resp
		}
		root, ok := msg.Args[2].(string)
		if !ok {
			resp <- makeErr(msg, errors.New("root argument is required"))
			close(resp)
			return resp
		}
		ch, errch, err := c.search(term, root)
		if err != nil {
			resp <- makeErr(msg, err)
			close(resp)
			return resp
		}
		go func() {
			for err := range errch {
				resp <- makeMessage("search.error", SearchResponse{
					ID:    id,
					Term:  term,
					Root:  root,
					Error: err.Error(),
				})
			}
		}()
		go func() {
			for r := range ch {
				resp <- makeMessage("search.output", SearchResponse{
					ID:      id,
					Term:    term,
					Root:    root,
					Results: []*finder.SearchResult{r},
				})
			}
			resp <- makeMessage("search.completed", SearchResponse{
				ID:   id,
				Term: term,
				Root: root,
			})
			close(resp)
		}()
		// reply the original message to signal that we are processsing it
		resp <- msg
		return resp
	}
	return resp
}

// fileFind returns a channel that will be populated with matched paths
// it will not return more results when we counted at least
// `fileFindLimit` matched files
//
// this returns
// - a channel for reading search results
// - a channel for reading potential errors happen in searching
// - an error when we failed to perform the search at all
func (c *Finder) fileFind(term, root string) (chan string, chan error, error) {
	var errSent bool // we only want to send error once
	errch := make(chan error, 1)
	ch := make(chan string, chanSize)

	fullpath, err := c.fs.Path(root)
	if err != nil {
		return nil, nil, err
	}
	f := finder.NewFinder(fullpath)
	results, err := f.FindFiles(term)
	if err != nil {
		return nil, nil, err
	}

	var counter int
	go func() {
		for r := range results {
			if counter < fileFindLimit {
				ch <- r
				counter++
			} else {
				if !errSent {
					errch <- fmt.Errorf("term(%s) has more than %d results", term, fileFindLimit)
					errSent = true
					close(errch)
				}
			}
		}
		close(ch)
	}()
	return ch, errch, nil
}

// search returns a channel that will be populated with matched contents
// it will not return more results when we counted at least
// `contentSearchLimit` matched lines
//
// this returns
// - a channel for reading search results
// - a channel for reading potential errors happen in searching
// - an error when we failed to perform the search at all
func (c *Finder) search(term, root string) (chan *finder.SearchResult, chan error, error) {
	var errSent bool // we only want to send error once
	errch := make(chan error, 1)
	ch := make(chan *finder.SearchResult, chanSize)

	fullpath, err := c.fs.Path(root)
	if err != nil {
		return nil, nil, err
	}
	f := finder.NewFinder(fullpath)
	results, err := f.FindContent(term)
	if err != nil {
		return nil, nil, err
	}

	var counter int
	go func() {
		for r := range results {
			if counter < contentSearchLimit {
				ch <- r
				counter += len(r.Matches)
			} else {
				if !errSent {
					errch <- fmt.Errorf("term(%s) has more than %d results", term, contentSearchLimit)
					errSent = true
					close(errch)
				}
			}
		}
		close(ch)
	}()
	return ch, errch, nil
}

func New(id string) (cops.Handler, error) {
	root := path.Join(RootPrefix, id)
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to find rootfs for %s, %s", id, err)
	}
	return &Finder{
		id: id,
		fs: rootfs.NewRootfs(root),
	}, nil
}

func init() {
	cops.RegisterHandler("finder", New)
}

func makeErr(orig *cops.Message, err error) *cops.Message {
	return &cops.Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Error:     err.Error(),
	}
}

func makeMessage(event string, args ...interface{}) *cops.Message {
	return &cops.Message{
		Namespace: "finder",
		Event:     event,
		Args:      args,
	}
}
