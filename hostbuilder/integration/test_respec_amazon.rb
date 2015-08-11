#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new
client.receive_timeout = 10000

if ARGV.length < 2
  puts "Usage: test_respec_amazon.rb <instance ID> <AWS region> <storage size> <instance type> <host ID> <host slug> <user ID>"
  puts "  Ya ikr so many params"
  exit 1
end

id = ARGV[0]
region = ARGV[1]
storage_size = ARGV[2].to_i
instance_type = ARGV[3]

host_id = ARGV[4]
host_slug = ARGV[5]
user_id = ARGV[6]

puts "Respeccing instance #{id}: storage size=#{storage_size}, instance type=#{instance_type}"

params = {
  id: id,
  region_id: region,
  storage_size: storage_size,
  disable_eip: false
}
if instance_type != ""
  params[:instance_type] = instance_type
end

resp = client.post "http://localhost:6666/hosts/respec", {
  provider: "amazon",
  id: host_id,
  slug: host_slug,
  uid: user_id,
  config: JSON.generate(params)
}
p resp
