#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"
require "securerandom"

client = HTTPClient.new
client.receive_timeout = 10000

random_id = SecureRandom.hex(8)
disable_eip = ARGV[0] == "true"

params = {
  name: "this-is-a-test-#{random_id}",
  source_ami: "ami-030f2333",
  region_id: "us-west-2",
  subnet_id: "subnet-3e0e045c",
  security_group_id: "sg-491c282c",
  instance_type: "t2.micro",
  storage_size: 8, # 8GB storage
  disable_eip: disable_eip,
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
