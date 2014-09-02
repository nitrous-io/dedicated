#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"
require "securerandom"

client = HTTPClient.new
client.receive_timeout = 10000

random_id = SecureRandom.hex(8)

params = {
  name: "test-box-#{random_id}",
  image_id: "5824829", # updated image with freighterd and docker
  region_id: "sgp", # Singapore 1
  size_id: "66", # 512MB
  public_keys: [
    File.read("#{ENV['HOME']}/.ssh/id_rsa.pub")
  ],
  apt_username: "dqminh",
  apt_password: "my_access_token",
  apt_location: "apt.nitrous.io",
  apt_dist: "trusty",
}

resp = client.post "http://localhost:6666/hosts", {
  provider: "digitalocean",
  config: JSON.generate(params)
}
p resp
