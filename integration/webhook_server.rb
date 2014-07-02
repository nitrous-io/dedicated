require "rubygems"
require "sinatra"
require "json"

post "/webhook" do
  request.body.rewind
  p "webhook info", params
  "ok"
end
