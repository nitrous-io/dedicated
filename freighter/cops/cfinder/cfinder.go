/*
cfinder is an cops interface to finder
*/
package cfinder

import (
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
	Paths []string `json:"paths"`
}

type SearchResponse struct {
	ID      string                 `json:"id"`
	Term    string                 `json:"term"`
	Root    string                 `json:"root"`
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
	id     string
	fs     *rootfs.Rootfs
	finder *finder.Finder
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
		ch, err := c.fileFind(term, root)
		if err != nil {
			resp <- makeErr(msg, err)
			close(resp)
			return resp
		}
		go func() {
			for r := range ch {
				resp <- makeFileFindMessage(&FileFindResponse{
					Term:  term,
					Root:  root,
					Paths: []string{r},
				})
			}
			close(resp)
		}()
		// reply the original message to signal that we are processsing it
		resp <- msg
		return resp
	case "search":
		id := msg.Args[0].(string)
		term := msg.Args[1].(string)
		root := msg.Args[2].(string)
		ch, err := c.search(term, root)
		if err != nil {
			resp <- makeErr(msg, err)
			close(resp)
			return resp
		}
		go func() {
			for r := range ch {
				resp <- makeSearchMessage(&SearchResponse{
					ID:      id,
					Term:    term,
					Root:    root,
					Results: []*finder.SearchResult{r},
				})
			}
			close(resp)
		}()
	}
	// reply the original message to signal that we are processsing it
	resp <- msg
	return resp
}

// fileFind returns a channel that will be populated with matched paths
// it will not return more results when we counted at least
// `fileFindLimit` matched files
func (c *Finder) fileFind(term, root string) (chan string, error) {
	ch := make(chan string, chanSize)
	f := finder.NewFinder(root)
	results, err := f.FindFiles(term)
	if err != nil {
		return nil, err
	}
	var counter int
	go func() {
		for r := range results {
			if counter < fileFindLimit {
				ch <- r
				counter++
			}
		}
		close(ch)
	}()
	return ch, nil
}

// search returns a channel that will be populated with matched contents
// it will not return more results when we counted at least
// `contentSearchLimit` matched lines
func (c *Finder) search(term, root string) (chan *finder.SearchResult, error) {
	ch := make(chan *finder.SearchResult, chanSize)
	f := finder.NewFinder(root)
	results, err := f.FindContent(term)
	if err != nil {
		return nil, err
	}
	var counter int
	go func() {
		for r := range results {
			if counter < contentSearchLimit {
				ch <- r
				counter += len(r.Matches)
			}
		}
		close(ch)
	}()
	return ch, nil
}

func NewFinder(id string) (cops.Handler, error) {
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
	cops.RegisterHandler("finder", NewFinder)
}

func makeErr(orig *cops.Message, err error) *cops.Message {
	return &cops.Message{
		Namespace: orig.Namespace,
		Event:     orig.Event,
		CID:       orig.CID,
		Error:     err,
	}
}

func makeFileFindMessage(r *FileFindResponse) *cops.Message {
	return &cops.Message{
		Namespace: "finder",
		Event:     "file_find.output",
		Args:      []interface{}{r},
	}
}

func makeSearchMessage(r *SearchResponse) *cops.Message {
	return &cops.Message{
		Namespace: "finder",
		Event:     "search.output",
		Args:      []interface{}{r},
	}
}
