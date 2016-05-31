require 'net/http'
require 'uri'

class Subscription

  def initialize
    #
  end

  def self.list_subscriptions(filter, ident)
    puts "#{filter.upcase} user subscriptions for #{ident}:"
    u = User.find(ident) {|ds| ds.where(users__deleted_at: nil)}

    # restrict output to those of only the latest token - needed if user
    # profile contains multiple login tokens
    tok = DB[:tokens].where(user_id: u.uid).order(Sequel.desc(:tokens__created_at)).first
    subs = Subscription.query.where(billing_profiles__user_id: u.uid).where(tokens__id: tok[:id])

    case filter
    when /active/
      subs = subs.where(subscriptions__state: "active")
    when /deleted/
      subs = subs.where(subscriptions__state: "deleted")
    when /all/          # all is same as no-filter (default) ... show both active/deleted
    else
      halt "Filter error: Not a valid filter type (all, active, deleted)"
    end

    # state, plans__id, plans__name, stripe_id, subscriptions__created_at
    halt "Subscriptions found: #{subs.count}" if subs.count < 1
    subs.all.each do |sub|
      puts <<-RESULTS
      Subscription information:
      .................... slug: #{sub[:subslug]}
      .................... user: #{sub[:username]}
      ................... state: #{sub[:state]}
      ............... plan_name: #{sub[:name]}
      .................... cost: #{sub[:price]}
      RESULTS
    end
  end

  def self.info(ident)
    puts "Getting info on specific customer subscription:"

    #subs = Subscription.query.where(subscriptions__slug: ident)
    subs = Subscription.query.where(subscriptions__slug: ident).
      order(Sequel.desc(:tokens__created_at)).first

    puts <<-RESULTS
    Subscription information:
    .................... slug: #{subs[:subslug]}
    .................... user: #{subs[:username]}
    ................... state: #{subs[:state]}
    ............... plan_name: #{subs[:name]}
    .................... cost: #{subs[:price]}
    ................ strip_id: #{subs[:stripe_id]}
    .............. created_at: #{subs[:subs_created_at]}
    ............ cancelled_at: #{subs[:subs_cancelled_at]}
    .... current_period_start: #{subs[:subs_current_period_start]}
    ...... current_period_end: #{subs[:subs_current_period_end]}
    RESULTS
  end

  def self.cancel(ident)
    puts "Attempting to cancel the following subscription:"

    subs = Subscription.query.where(subscriptions__slug: ident)

    # there are multiple tokens per user, so make the most query result with
    # the most recently created token at first position in the list
    s = subs.where(subscriptions__state: "active").
              order(Sequel.desc(:tokens__created_at))

    # ensure active subscription AND make query only with the most recently-created token
    halt "Cancel FAILED -- not an active subscription" if s.count < 1
    sub = s.all[0]

    puts "....... ident: #{ident}"
    puts ".... username: #{sub[:username]}"

    token = sub[:token]
    req_object = Net::HTTP::Delete
    Subscription.request(ident, "subscriptions", "cancel", token, req_object)
  end

  # Set up a net::http query to the subscriptions resource
  def self.request(ident, resource, action, token, req_object)
    base_url = 'https://www.n2ostaging.com/api/v0'
    url = base_url + '/' + resource + '/' + ident + '/' + action

    uri = URI.parse(url)
    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = true
    http.verify_mode = OpenSSL::SSL::VERIFY_NONE
    # passing in (vs hard-coded) req_object allows re-use for GET/POST/DELETE/etc ...
    req = req_object.new(uri.request_uri)
    req["Authorization"] = "Bearer #{token}"

    res = http.request(req)

    if res.code.eql? "200"
      puts "Cancellation: Succeeded"
    end
  end

  # Set up the main query to join subscriptions, plans, and users tables
  def self.query
    DB[:users].
      select_all(:users, :plans, :subscriptions, :tokens).
      select_more(:users__id___uid).
      select_more(:subscriptions__slug___subslug).
      select_more(:subscriptions__created_at___subs_created_at).
      select_more(:subscriptions__cancelled_at___subs_cancelled_at).
      select_more(:subscriptions__current_period_start___subs_current_period_start).
      select_more(:subscriptions__current_period_end___subs_current_period_end).
      left_join(:billing_profiles, user_id: :users__id).
      left_join(:subscriptions, billing_profile_id: :billing_profiles__id).
      left_join(:plans, id: :subscriptions__plan_id).
      left_join(:tokens, user_id: :users__id)
  end
end

# vim:ft=ruby
