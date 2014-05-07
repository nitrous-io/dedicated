#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"
require "securerandom"

client = HTTPClient.new
client.receive_timeout = 10000

random_id = SecureRandom.hex(32)

params = {
  name: "test-box-#{random_id}",
  image_id: "3240036", # Ubuntu 14.04 x64
  region_id: "6", # Singapore 1
  size_id: "66", # 512MB
  public_keys: [
    File.read("#{ENV['HOME']}/.ssh/id_rsa.pub")
  ],
}

resp = client.post "http://localhost:6666/host", {
  provider: "digitalocean",
  config: JSON.generate(params)
}
p resp
