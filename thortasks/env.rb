require 'sequel'

DB = Sequel.connect(ENV['NITROUS_PG_URL'])
