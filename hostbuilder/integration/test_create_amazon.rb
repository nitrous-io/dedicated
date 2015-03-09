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
  name: "this-is-a-test-#{random_id}",
  source_ami: "ami-5d98b86d",
  region_id: "us-west-2",
  security_group_id: "sg-c4b2f0f7",
  instance_type: "t1.micro",
  storage_size: 5, # 50GB storage
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
  secret: "not-a-secret",
}

resp = client.post "http://localhost:6666/hosts", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
