/*
cfinder is an cops interface to finder.

This supports file-find and search in a directory.

File-find returns a list of file paths that match the given term. Its message
format is:

--> d|{ "namespace":"finder", "event: "file_find", "cid": 42, "args": {"term":"asd", "root":"/home/action"} }
<-- d|{ "namespace":"finder", "event: "file_find", "cid": 42, "args": {"term":"asd", "root":"/home/action"} } # signal that we received the message
<-- d|{ "namespace":"finder", "event: "file_find.output", "args": {"term":"asd", "root":"/home/action", "paths": ["workspace/asd"]} }
<-- d|{ "namespace":"finder", "event: "file_find.error", "args": {"term":"asd", "root":"/home/action", "error": "some error"}} # error occurs in the middle of file-find op
<-- d|{ "namespace":"finder", "event: "file_find.completed", "args": {"term":"asd", "root":"/home/action"}} # file find completed

--> d|{ "namespace":"finder", "event: "file_find", "cid": 43, "args": {"term":"another", "root":"/home/action"} } # another request
<-- d|{ "namespace":"finder", "event: "file_find", "cid": 43, "error": "failed to find file" } # if an error occurs before the file-find op starts

Search returns a list of files that contain the given term. Each file includes
metadata that indicates where the given term was matched (i.e. line number and
surrounding content). Its message format is:

--> d|{ "namespace":"finder", "event: "search", "cid": 44, "args": [{ "id": "123", "term": "asd", "root": "/home/action/workspace", "ignore_case": true, "use_regexp": false }] } # args is a single item array of a hash with keys: id, term, root, ignore_case, use_regexp
<-- d|{ "namespace":"finder", "event: "search", "cid": 44, "args": [{ "id": "123", "term": "asd", "root": "/home/action/workspace", "ignore_case": true, "use_regexp": false }] }
<-- d|{ "namespace":"finder", "event: "search.output", "args": [{ "request": {"id": "123", "term":"asd", "root":"/home/action", "ignore_case": true, "use_regexp": false, "results": { "file":"filename", "matches": [{"row": 12, "content": "line asd"}] } }] }
<-- d|{ "namespace":"finder", "event: "search.error", "args": [{ "request": { "id": "123", "term": "asd", "root": "/home/action/workspace", "ignore_case": true, "use_regexp": false }, "error": "some error"}] } # error occurs in the middle of search op
<-- d|{ "namespace":"finder", "event: "search.completed", "args": [{ "request": { "id": "123", "term": "asd", "root": "/home/action/workspace", "ignore_case": true, "use_regexp": false }}] } # search completed

--> d|{ "namespace":"finder", "event: "search", "cid": 45, "args": [{ "id": "123", "term": "another", "root": "/home/action"] }
<-- d|{ "namespace":"finder", "event: "search", "cid": 45, "error": "filesystem disappeared" } # if an error occurs before the search op starts
*/
package cops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nitrous-io/dedicated/pkg/finder"
)

var (
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

type SearchRequest struct {
	ID          string `json:"id"`
	Term        string `json:"term"`
	Root        string `json:"root"`
	IgnoreCase  bool   `json:"ignore_case"`
	UseRegexp   bool   `json:"use_regexp"`
	FilePattern string `json:"file_pattern"`
}

type SearchResponse struct {
	SearchRequest `json:"request"`

	Error   string                 `json:"error"`
	Results []*finder.SearchResult `json:"results"`
}

// The flow of response from Finder is
//
// --> d|{"event": "file_find", "cid": 1, ...}
// <-- d|{"event": "file_find", "cid": 1, ...} // this signal that we are processing it
// <-- d|{"event": "file_find.output", args: [{"paths": ["container.go"], ..}], ...}
//
// the client should catch `{{event}}.output` content and pring output
// accordingly
type Finder struct {
}

func doFileFind(msg *Message, resp chan *Message) chan *Message {
	term := msg.Args[0].(string)
	dir := msg.Args[1].(string)

	fileinfo, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return sendErr(resp, msg, fmt.Errorf("%q does not exist", dir))
	}
	if !fileinfo.IsDir() {
		return sendErr(resp, msg, fmt.Errorf("%q is not a directory", dir))
	}

	// reply the original message to signal that we are processsing it
	resp <- msg

	go func() {
		matches, err := finder.FindFiles(dir, term)
		if err != nil {
			sendErr(resp, msg, err)
			return
		}
		if len(matches) > fileFindLimit {
			matches = matches[:fileFindLimit]
		}

		for _, match := range matches {
			relativePath := strings.TrimPrefix(match.FilePath, dir)
			// dir might or might not include "/" at the end
			relativePath = strings.TrimPrefix(relativePath, "/")
			sendMessage(resp, msg, "file_find.output", FileFindResponse{
				Term:  term,
				Root:  dir,
				Paths: []string{relativePath},
			})
		}
		sendFinalMessage(resp, msg, "file_find.completed", FileFindResponse{
			Term: term,
			Root: dir,
		})
	}()

	return resp
}

func (c *Finder) HandleAsync(msg *Message) chan *Message {
	resp := make(chan *Message, chanSize)
	switch msg.Event {
	case "file_find":
		return doFileFind(msg, resp)

	case "search":
		if len(msg.Args) < 1 {
			return sendErr(resp, msg, errors.New("request argument is required"))
		}
		arg, ok := msg.Args[0].(map[string]interface{})
		if !ok {
			return sendErr(resp, msg, fmt.Errorf("request argument %+v is in invalid format", msg.Args[0]))
		}

		var req SearchRequest
		err := mapToStruct(arg, &req)
		if err != nil {
			return sendErr(resp, msg, fmt.Errorf("request argument %+v is in invalid format", arg))
		}

		ch, errch, err := c.search(req.Term, req.Root, req.IgnoreCase, req.UseRegexp, req.FilePattern)
		if err != nil {
			return sendErr(resp, msg, err)
		}

		go func() {
			for err := range errch {
				sendMessage(resp, msg, "search.error", SearchResponse{
					SearchRequest: req,
					Error:         err.Error(),
				})
			}
		}()
		go func() {
			for r := range ch {
				sendMessage(resp, msg, "search.output", SearchResponse{
					SearchRequest: req,
					Results:       []*finder.SearchResult{r},
				})
			}
			sendFinalMessage(resp, msg, "search.completed", SearchResponse{
				SearchRequest: req,
			})
		}()

		// reply the original message to signal that we are processsing it
		resp <- msg
		return resp
	}
	return resp
}

// search returns a channel that will be populated with matched contents
// it will not return more results when we counted at least
// `contentSearchLimit` matched lines
//
// this returns
// - a channel for reading search results
// - a channel for reading potential errors that happen during searching
// - an error when we fail to perform the search at all
func (c *Finder) search(term, root string, ignoreCase, useRegexp bool, filePattern string) (chan *finder.SearchResult, chan error, error) {
	var errSent bool // we only want to send error once
	errch := make(chan error, 1)
	ch := make(chan *finder.SearchResult, chanSize)

	fileinfo, err := os.Stat(root)
	if os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("%q does not exist", root)
	}
	if !fileinfo.IsDir() {
		return nil, nil, fmt.Errorf("%q is not a directory", root)
	}

	f := finder.NewFinder(root)
	filePattern = finder.SanitizeFilePattern(filePattern)
	results, err := f.FindContent(term, ignoreCase, useRegexp, filePattern)
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
					errch <- fmt.Errorf("term %q has more than %d results", term, contentSearchLimit)
					errSent = true
					close(errch)
				}
			}
		}
		close(ch)
	}()
	return ch, errch, nil
}

func NewFinderHandler() AsyncHandler {
	return &Finder{}
}

// mapToStruct converts a map[string]interface{} to a struct. The given struct
// value must be a pointer to a struct.
// This is a wasteful conversion (but an acknowledged trade-off for ease of
// use).
func mapToStruct(m map[string]interface{}, val interface{}) error {
	tmp, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(tmp, val)
}
