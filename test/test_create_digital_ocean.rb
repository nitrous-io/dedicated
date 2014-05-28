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
  #image_id: "3240036", # Ubuntu 14.04 x64
  image_id: "4121892", # test trusty image
  region_id: "6", # Singapore 1
  size_id: "66", # 512MB
  public_keys: [
    File.read("#{ENV['HOME']}/.ssh/id_rsa.pub")
  ],
  apt_username: "dqminh",
  apt_password: "my_access_token",
  apt_location: "apt.nitrous.io",
  apt_dist: "trusty",
}

resp = client.post "http://localhost:6666/host", {
  provider: "digitalocean",
  webhook: "http://127.0.0.1:4567/webhook",
  config: JSON.generate(params)
}
p resp
