#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new
client.receive_timeout = 10000

if ARGV.length < 2
  puts "Usage: test_respec_amazon.rb <instance ID> <storage size>"
  exit 1
end

id = ARGV[0]
storage_size = ARGV[1].to_i

puts "Respeccing instance #{id} with storage size: #{storage_size}..."

params = {
  id: id,
  region_id: "ap-southeast-1",
  storage_size: storage_size,
  disable_eip: false
}

resp = client.post "http://localhost:6666/hosts/respec", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
