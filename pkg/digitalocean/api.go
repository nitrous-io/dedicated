package digitalocean

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DIGITALOCEAN_API_URL = "https://api.digitalocean.com"

type (
	SSHKey struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		PubKey string `json:"ssh_pub_key"`
	}

	SSHKeyResp struct {
		Status string  `json:"status"`
		Key    *SSHKey `json:"ssh_key"`
	}

	SSHKeysResp struct {
		Status string    `json:"status"`
		Keys   []*SSHKey `json:"ssh_keys"`
	}

	Image struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		Distribution string `json:"distribution"`
	}

	ImageResp struct {
		Status string `json:"status"`
		Image  *Image `json:"image"`
	}

	ImagesResp struct {
		Status string   `json:"status"`
		Images []*Image `json:"images"`
	}

	Region struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	RegionResp struct {
		Status string  `json:"status"`
		Region *Region `json:"region"`
	}

	RegionsResp struct {
		Status  string    `json:"status"`
		Regions []*Region `json:"regions"`
	}

	Size struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	SizesResp struct {
		Status string  `json:"status"`
		Sizes  []*Size `json:"sizes"`
	}

	GenericResp struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}

	DropletResp struct {
		Status   string `json:"status"`
		*Droplet `json:"droplet"`
	}

	DropletsResp struct {
		Status   string     `json:"status"`
		Droplets []*Droplet `json:"droplets"`
	}

	Droplet struct {
		ID        int       `json:"id"`
		ImageID   int       `json:"image_id"`
		SizeID    int       `json:"size_id"`
		RegionID  int       `json:"region_id"`
		Name      string    `json:"name"`
		IpAddress string    `json:"ip_address"`
		Locked    bool      `json:"locked"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}

	Client struct {
		client   *http.Client // The http client for communicating
		BaseURL  string       // The base URL of the API
		ClientID string
		APIKey   string
	}
)

var regionsMap = map[string]string{
	"sgp": "sgp1",
	"nyc": "nyc2",
	"sfo": "sfo1",
	"ams": "ams2",
	"lon": "lon1",
}

// ErrAccessDenied is returned when DO responds with 401 status code
var ErrAccessDenied = errors.New("access denied")

// ErrNotFound is returned when DO responds with 404 status code
var ErrNotFound = errors.New("resource not found")

// Creates a new client for communicating with DO
// To fake the server, replace BaseURL with the desired URL after the client is
// created
func NewClient(client string, key string) *Client {
	c := &Client{
		client: &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
			},
		},
		BaseURL:  DIGITALOCEAN_API_URL,
		ClientID: client,
		APIKey:   key,
	}
	return c
}

// Creates an SSH Key and returns it's id
func (d Client) CreateKey(name string, pub string) (*SSHKey, error) {
	params := url.Values{}
	params.Set("name", name)
	params.Set("ssh_pub_key", pub)

	body, err := NewRequest(d, "ssh_keys/new", params)
	if err != nil {
		return nil, err
	}
	var resp SSHKeyResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Key, nil
}

// Destroys an SSH key
func (d Client) DestroyKey(id int) error {
	path := fmt.Sprintf("ssh_keys/%v/destroy", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// Creates a droplet and returns it's id
func (d Client) CreateDroplet(
	name string,
	rawSize string,
	rawImage string,
	rawRegion string,
	keyID int,
	privateNetworking bool) (*Droplet, error) {
	params := url.Values{}
	params.Set("name", name)

	size, err := d.Size(rawSize)
	if err != nil {
		return nil, fmt.Errorf("Invalid size or lookup failure: '%s': %s", rawSize, err)
	}

	image, err := d.Image(rawImage)
	if err != nil {
		return nil, fmt.Errorf("Invalid image or lookup failure: '%s': %s", rawImage, err)
	}

	normalizedRegion, ok := regionsMap[rawRegion]
	if !ok {
		return nil, fmt.Errorf("Invalid region or lookup failure: '%s': %s", rawRegion, err)
	}

	region, err := d.Region(normalizedRegion)
	if err != nil {
		return nil, fmt.Errorf("Invalid region or lookup failure: '%s': %s", rawRegion, err)
	}

	params.Set("size_id", strconv.Itoa(size.ID))
	params.Set("image_id", strconv.Itoa(image.ID))
	params.Set("region_id", strconv.Itoa(region.ID))
	params.Set("ssh_key_ids", fmt.Sprintf("%v", keyID))
	params.Set("private_networking", fmt.Sprintf("%v", privateNetworking))

	body, err := NewRequest(d, "droplets/new", params)
	if err != nil {
		return nil, err
	}
	var resp DropletResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Droplet, nil
}

// Destroys a droplet
func (d Client) DestroyDroplet(id int) error {
	path := fmt.Sprintf("droplets/%v/destroy", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// Powers off a droplet
func (d Client) PowerOffDroplet(id int) error {
	path := fmt.Sprintf("droplets/%v/power_off", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// Powers on a droplet
func (d Client) PowerOnDroplet(id int) error {
	path := fmt.Sprintf("droplets/%v/power_on", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// Stop a droplet and turn it back on
func (d Client) PowerCycleDroplet(id int) error {
	path := fmt.Sprintf("droplets/%v/power_cycle", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// Reboot a droplet. This is the preferred method to use if a server is not
// responding.
func (d Client) RebootDroplet(id int) error {
	path := fmt.Sprintf("droplets/%v/reboot", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// Shutsdown a droplet. This is a "soft" shutdown.
func (d Client) ShutdownDroplet(id int) error {
	path := fmt.Sprintf("droplets/%v/shutdown", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// Creates a snaphot of a droplet by it's ID
func (d Client) CreateSnapshot(id int, name string) error {
	path := fmt.Sprintf("droplets/%v/snapshot", id)
	params := url.Values{}
	params.Set("name", name)
	_, err := NewRequest(d, path, params)
	return err
}

// Restores a snapshot of a droplet by it's ID
func (d Client) RestoreSnapshot(id int, imageID int) error {
	path := fmt.Sprintf("droplets/%v/restore", id)
	params := url.Values{}
	params.Set("image_id", strconv.Itoa(imageID))
	_, err := NewRequest(d, path, params)
	return err
}

// Returns all available images.
func (d Client) Images() ([]*Image, error) {
	body, err := NewRequest(d, "images", url.Values{})
	if err != nil {
		return nil, err
	}
	var resp ImagesResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Images, nil
}

// Destroys an image by its ID.
func (d Client) DestroyImage(id int) error {
	path := fmt.Sprintf("images/%d/destroy", id)
	_, err := NewRequest(d, path, url.Values{})
	return err
}

// DropletInfo returns the information of the droplet with the given id.
func (d Client) DropletInfo(id int) (*Droplet, error) {
	body, err := NewRequest(d, fmt.Sprintf("droplets/%v", id), url.Values{})
	if err != nil {
		return nil, err
	}
	var resp DropletResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Droplet, nil
}

// Sends an api request and returns []byte for the response if possible
func NewRequest(d Client, path string, params url.Values) ([]byte, error) {
	client := d.client

	// Add the authentication parameters
	params.Set("client_id", d.ClientID)
	params.Set("api_key", d.APIKey)

	url := fmt.Sprintf("%s/%s?%s", d.BaseURL, path, params.Encode())

	// Do some basic scrubbing so sensitive information doesn't appear in logs
	scrubbedUrl := strings.Replace(url, d.ClientID, "CLIENT_ID", -1)
	scrubbedUrl = strings.Replace(scrubbedUrl, d.APIKey, "API_KEY", -1)
	log.Printf("sending new request to digitalocean: %s", scrubbedUrl)

	var lastErr error
	for attempts := 1; attempts < 10; attempts++ {
		resp, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, ErrAccessDenied
		} else if resp.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}

		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}

		log.Printf("response from digitalocean: %s", body)

		genericResp := &GenericResp{}
		if err = json.Unmarshal(body, genericResp); err != nil {
			err = fmt.Errorf("Failed to decode response (%v): %s",
				resp.StatusCode, body)
			log.Println(err)
			return body, err
		}

		// Check for errors sent by digitalocean
		switch genericResp.Status {
		case "OK":
			return body, nil
		case "ERROR":
			err := fmt.Errorf("Error: %s. Full response body: %s",
				genericResp.Message, body)
			return body, err
		}

		lastErr = fmt.Errorf("Received error from DigitalOcean (%d): %s",
			resp.StatusCode, genericResp.Status)
		log.Println(lastErr)
		if strings.Contains(genericResp.Status, "a pending event") {
			// Retry, DigitalOcean sends these dumb "pending event"
			// errors all the time.
			time.Sleep(5 * time.Second)
			continue
		}
		// Some other kind of error. Just return.
		return body, lastErr
	}

	return nil, lastErr
}

func (d Client) Image(slugOrID string) (*Image, error) {
	body, err := NewRequest(d, fmt.Sprintf("images/%v", slugOrID), url.Values{})
	if err != nil {
		return nil, err
	}
	var resp ImageResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Image, nil
}

// Returns all available regions.
func (d Client) Regions() ([]*Region, error) {
	resp, err := NewRequest(d, "regions", url.Values{})
	if err != nil {
		return nil, err
	}
	result := &RegionsResp{}
	if err := json.Unmarshal(resp, result); err != nil {
		return nil, err
	}
	return result.Regions, nil
}

func (d Client) Region(slugOrID string) (*Region, error) {
	regions, err := d.Regions()
	if err != nil {
		return nil, err
	}
	for _, region := range regions {
		if strings.EqualFold(region.Slug, slugOrID) {
			return region, nil
		}
	}
	for _, region := range regions {
		id, err := strconv.Atoi(slugOrID)
		if err == nil {
			if region.ID == id {
				return region, nil
			}
		}
	}
	return nil, fmt.Errorf("Unknown region '%v'", slugOrID)
}

// Returns all available sizes.
func (d Client) Sizes() ([]*Size, error) {
	resp, err := NewRequest(d, "sizes", url.Values{})
	if err != nil {
		return nil, err
	}
	result := &SizesResp{}
	if err := json.Unmarshal(resp, result); err != nil {
		return nil, err
	}
	return result.Sizes, nil
}

func (d Client) Size(slugOrID string) (*Size, error) {
	sizes, err := d.Sizes()
	if err != nil {
		return nil, err
	}
	for _, size := range sizes {
		if strings.EqualFold(size.Slug, slugOrID) {
			return size, nil
		}
		id, err := strconv.Atoi(slugOrID)
		if err == nil {
			if size.ID == id {
				return size, nil
			}
		}
	}
	return nil, fmt.Errorf("Unknown size %v", slugOrID)
}
