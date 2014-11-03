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
  zone: "asia-east1-a",
  image: "packer-1415162758",
  machine_type: "f1-micro",
  public_keys: [
    File.read("#{ENV['HOME']}/.ssh/id_rsa.pub")
  ],
  apt_username: "dqminh",
  apt_password: "my_access_token",
  apt_location: "apt.nitrous.io",
  apt_dist: "trusty",
  tls: {
    ca: "tls-ca",
    cert: "tls-cert",
    key: "tls-key"
  },
  secret: "not-a-secret"
}

resp = client.post "http://localhost:6666/hosts", {
  provider: "gce",
  config: JSON.generate(params)
}
p resp
