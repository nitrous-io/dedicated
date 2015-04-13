// This package implements the Mixpanel API as referenced here: https://mixpanel.com/help/reference/http
package mixpanel

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

const BASE_URL = "https://api.mixpanel.com"

var (
	// This error is returned when Mixpanel returns a non-success message when tracking an event
	ErrUnexpectedTrackResponse = fmt.Errorf("Unexpected Mixpanel Track Response")
	// This error is returned when Mixpanel returns a non-success message when using an engage event
	ErrUnexpectedEngageResponse = fmt.Errorf("Unexpected Mixpanel Engage Response")
)

type Mixpanel struct {
	Token   string
	BaseURL string
}

// a ProfileOp is the information needed for a single operation on a profile.
// While multiple operations can be run with an unchanged ProfileOp, this is
// usually not desired.
type ProfileOp struct {
	*Mixpanel
	DistinctID string
	Properties map[string]interface{}
}

// NewMixpanelClient returns a Mixpanel struct with which you can perform other Mixpanel operations
// e.g. `m := mixpanel.NewMixpanelClient("your_mixpanel_token")`
func NewMixpanelClient(args ...string) *Mixpanel {
	var m *Mixpanel

	if len(args) == 1 {
		m = &Mixpanel{Token: args[0], BaseURL: BASE_URL}
	} else if len(args) > 1 {
		m = &Mixpanel{Token: args[0], BaseURL: args[1]}
	}

	return m
}

// NewProfileOp returns a ProfileOp struct ready for profile operations
func NewProfileOp(m *Mixpanel, distinctID string, props map[string]interface{}) *ProfileOp {
	return &ProfileOp{
		Mixpanel:   m,
		DistinctID: distinctID,
		Properties: props,
	}
}

// NewProfileOp returns a ProfileOp struct ready for profile operations
func (m *Mixpanel) NewProfileOp(distinctID string, props map[string]interface{}) *ProfileOp {
	return NewProfileOp(m, distinctID, props)
}

// Track creates a Mixpanel event for the "event" string along with other properties
// that are added to the event as meta-data
// e.g. `err := mc.Track("User Signed Up", map[string]interface{}{"$distinct_id": "1"})`
func (m *Mixpanel) Track(event string, properties map[string]interface{}) error {
	var data map[string]interface{} = make(map[string]interface{})

	data["event"] = event
	properties["token"] = m.Token
	data["properties"] = properties

	response, err := m.get(fmt.Sprintf("%s/track/", m.BaseURL), data)
	if err != nil {
		return err
	}

	if response != "1" {
		return ErrUnexpectedTrackResponse
	}

	return nil
}

// Set updates the profile with the profile op's properties
func (p *ProfileOp) Set() error {
	return p.engage(p.DistinctID, "$set", p.Properties)
}

// Create is an alias for set (which creates a profile in Mixpanel if
// distinctID does not exist).  Use it to communicate intent.
func (p *ProfileOp) Create() error {
	return p.Set()
}

// SetOnce updates the profile with the profile op's properties if they are not
// already set
func (p *ProfileOp) SetOnce() error {
	return p.engage(p.DistinctID, "$set_once", p.Properties)
}

// Increment updates the profile by incrementing each of the keys in the
// profile op's properties with their corresponding value.  Decrement is
// supported by using negative values.
func (p *ProfileOp) Increment() error {
	return p.engage(p.DistinctID, "$add", p.Properties)
}

// Append updates the profile by appending to each property instead of
// overwriting it.  Append with an absent property will turn it into a
// one-element list.
func (p *ProfileOp) Append() error {
	return p.engage(p.DistinctID, "$append", p.Properties)
}

// Union unions values to the given properties of the profile
// that is referenced by the distinctID (which is the primary key)
// e.g. `err := m.UnionPropertiesOnProfileOp("1", map[string]interface{}{"items_purchased": []string{"socks", "shirts"}})`
func (p *ProfileOp) Union() error {
	return p.engage(p.DistinctID, "$union", p.Properties)
}

// Unset removes properties from a profile
func (p *ProfileOp) Unset(keys []string) error {
	return p.engage(p.DistinctID, "$unset", keys)
}

// Delete deletes a profile
func (p *ProfileOp) Delete() error {
	return p.engage(p.DistinctID, "$delete", "")
}

// Alias adds an alias for the profile's distinctID
func (p *ProfileOp) Alias(newID string) error {
	return p.Track("$create_alias", map[string]interface{}{"distinct_id": p.DistinctID, "alias": newID})
}

// engage sends the requests to Mixpanel.  It recognizes special properties and
// promotes them to the top level.  These are:
// * $ignore_time : bool, default false .  Set to true to avoid updating Last Seen
// * $ip : string . Set to "0" to tell mixpanel to ignore IP information.
//   Leave empty to use the IP address of the request
func (m *Mixpanel) engage(distinctID string, op string, properties interface{}) error {
	var data map[string]interface{} = make(map[string]interface{})

	data["$token"] = m.Token
	data["$distinct_id"] = distinctID

	// promote special properties if found
	if m, ok := properties.(map[string]interface{}); ok {
		if ip, found := m["$ip"]; found {
			data["$ip"] = ip
			delete(m, "$ip")
		}
		if ignore_time, found := m["$ignore_time"]; found {
			data["$ignore_time"] = ignore_time
			delete(m, "$ignore_time")
		}
	}

	data[op] = properties

	response, err := m.get(fmt.Sprintf("%s/engage/", m.BaseURL), data)
	if err != nil {
		return err
	}

	if response != "1" {
		return ErrUnexpectedEngageResponse
	}

	return nil
}

func (m *Mixpanel) get(url string, data map[string]interface{}) (string, error) {
	jsonedData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	base64JSONData := base64.StdEncoding.EncodeToString(jsonedData)

	res, err := http.Get(fmt.Sprintf("%s?data=%s", url, base64JSONData))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	responseBody, err := ioutil.ReadAll(res.Body)

	return string(responseBody), err
}
