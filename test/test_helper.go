package test

import (
	. "github.com/nitrous-io/hostbuilder/pkg/digitalocean"
)

var (
	// /droplets
	AllDroplets = `
	{
		"status": "OK",
		"droplets": [
			{
				"id": 100823,
				"name": "test222",
				"image_id": 420,
				"size_id":33,
				"region_id": 1,
				"backups_active": false,
				"ip_address": "127.0.0.1",
				"private_ip_address": null,
				"locked": false,
				"status": "active",
				"created_at": "2013-01-01T09:30:00Z"
			}
		]
	}
	`

	// /droplets/new
	NewDroplet = `
	{
		"status": "OK",
		"droplet": {
			"id": 100823,
			"name": "test222",
			"image_id": 420,
			"size_id":33,
			"region_id": 1
		}
	}
	`

	// /droplets/id
	NewDropletState = `
	{
		"status": "OK",
		"droplet": {
			"id": 100823,
			"image_id": 420,
			"name": "test222",
			"region_id": 1,
			"size_id": 33,
			"ip_address": "127.0.0.1",
			"locked": false,
			"status": "new"
		}
	}
	`

	ActiveDropletState = `
	{
		"status": "OK",
		"droplet": {
			"id": 100823,
			"image_id": 420,
			"name": "test222",
			"region_id": 1,
			"size_id": 33,
			"ip_address": "127.0.0.1",
			"locked": false,
			"status": "active"
		}
	}
	`

	// /regions
	AllRegions = `
	{
		"status": "OK",
		"regions": [
			{ "id": 1, "name": "New York 1", "slug": "nyc1" },
			{ "id": 2, "name": "Amsterdam 1", "slug": "ams1" },
			{ "id": 3, "name": "San Francisco 1", "slug": "sfo1" }
		]
	}
	`

	// /sizes
	AllSizes = `
	{
		"status": "OK",
		"sizes": [
			{ "id": 33, "name": "512MB", "slug": "512mb" },
			{ "id": 34, "name": "1GB", "slug": "1gb" },
			{ "id": 35, "name": "2GB", "slug": "2gb" },
			{ "id": 36, "name": "4GB", "slug": "4gb" },
			{ "id": 37, "name": "8GB", "slug": "8gb" },
			{ "id": 38, "name": "16GB", "slug": "16gb" }
		]
	}
	`

	// /images
	AllImages = `
	{
		"status": "OK",
		"images": [
			{ "id": 1, "name": "Snapshot", "distribution": "Ubuntu", "slug": "ubuntu-12.04-x64", "public": true },
			{ "id": 2, "name": "Automated Backup", "distribution": "Ubuntu" }
		]
	}
	`

	// /image/id
	OneImage = `
	{
		"status": "OK",
		"image": {
			"id": 1,
			"name": "Automated Backup",
			"distribution":"Ubuntu"
		}
	}
	`

	// /ssh_keys/new
	NewSSH = `
	{
		"status": "OK",
		"ssh_key": {
			"id": 47,
			"name": "my_key",
			"ssh_pub_key": "ssh-rsa key user@host"
		}
	}
	`
)

func TestClient(url string) *Client {
	client := NewClient("client_id", "api_key")
	client.BaseURL = url
	return client
}
