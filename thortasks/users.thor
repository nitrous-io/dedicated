require 'securerandom'

class Users < Thor
  desc "info IDENT",  "Outputs information on single user, where IDENT is id, username, or email"
  def info(ident)
    puts User.find(ident)
  end

  desc "to_ban", "Lists suspicious users that are not banned"
  def to_ban
    results = User.query.
      exclude(suspicious: nil).where(banned_at: nil).
      order(Sequel.desc(:users__id))
    puts "Too many results (#{results.count}). Displaying most recent 5" if results.count > 5
    results.to_a.take(5).each do |r|
      puts "-" * 20
      puts User.new(r)
    end
  end

  desc "unsuspicious IDENT", "Makes user IDENT no longer suspicious"
  def unsuspicious(ident)
    u = User.find(ident) {|ds| ds.where(users__deleted_at: nil) }
    halt "User #{u.username} is not suspicious" if !u.suspicious
    old_suspicious = u.suspicious
    NITROUS_DB.transaction do
      r = NITROUS_DB[:users].where(id: u.uid).update(suspicious: nil)
      rows_expected!(r, 1)
      puts "#{u.summary}: suspicious '#{old_suspicious}' => null"
      if u.bp_state == 'delinquent'
        # users from brazil and turkey are set to delinquent when adding their card
        # we need to change them back to 'paid' state
        r = NITROUS_DB[:billing_profiles].where(user_id: u.uid).update(state: 'paid')
        puts "#{u.summary}: bp state 'delinquent' => 'paid'"
      end
    end
  end

  # Should only be used on bonuses that are not granted automatically by the server
  desc "grant_bonus IDENT BONUS [SECONDS]",  "Grants bonus to user by adding to usage_quota and inserting earned_bonus record"
  def grant_bonus(ident, bonus, amount=nil)
    u = User.find(ident) {|ds| ds.where(users__deleted_at: nil) }
    NITROUS_DB.transaction do
      bn = NITROUS_DB[:bonuses].where(slug: bonus).first
      bonusAmount = amount || bn[:amount]
      r = NITROUS_DB[:users].where(id: u.uid).update(usage_quota: u.usage_quota + bonusAmount.to_i)
      rows_expected!(r, 1)
      u1 = User.find(ident) {|ds| ds.where(users__deleted_at: nil) }
      begin
        NITROUS_DB[:earned_bonuses].insert(bonus_id: bn[:id], user_id: u.uid)
        puts "#{u.summary}:"
        puts "\tusage_quota updated from #{u.usage_quota} to #{u1.usage_quota}"
        puts "\tearned_bonuses record inserted"
      rescue Sequel::UniqueConstraintViolation
        puts "#{u.summary} has already earned bonus '#{bonus}'"
      end
    end
  end

  desc "search TERM", "Searches user usernames and e-mails for TERM"
  def search(term)
    term = term.gsub('_', '\_') # escape for LIKE
    term = ['%', term, '%'].join if !term.match(/%/) # add wildcards for substring if wildcards not present
    results = User.query.
      where { Sequel.like(:username, term) | Sequel.like(:email, term) }.
      order(Sequel.desc(:users__id))
    puts "Too many results (#{results.count}). Displaying most recent 5" if results.count > 5
    results.to_a.take(5).each do |r|
      puts "-" * 20
      puts User.new(r)
    end
  end

  desc "to_cancel", "Lists users whose subscriptions are eligible to cancel"
  def to_cancel
    rs = NITROUS_DB[:users].distinct(:subscriptions__updated_at).
      select_more(:billing_profiles__stripe_id___customer_id).
      select_more(:subscriptions__stripe_id___sub_id).
      join(:billing_profiles, user_id: :users__id).
      join(:hosts, user_id: :users__id).
      join(:subscriptions, id: :hosts__subscription_id).
      where(subscriptions__state: 'unpaid').
      where(hosts__terminated_at: nil).
      order(:subscriptions__updated_at)
    puts "Failed subscriptions:"
    max_username = rs.map {|r| r[:username] }.max_by(&:size).size
    max_email = rs.map {|r| r[:email] }.max_by(&:size).size
    rs.each do |r|
      printf "%-#{max_username}s | %-#{max_email}s | %14s | %14s\n", r[:username], r[:email], r[:customer_id], r[:sub_id]
    end
  end

  desc "flatiron_token IDENT", "Returns most recent token for flatiron user IDENT. Creates token if none present"
  def flatiron_token(ident)
    u = User.find(ident) {|ds| ds.where(users__deleted_at: nil) }
    r = NITROUS_DB[:orgs].
      select_more(:users__id___uid).
      join(:org_memberships, org_id: :orgs__id).
      join(:users, id: :org_memberships__user_id).
      where(orgs__deleted_at: nil).
      where(orgs__slug: 'flatiron-org').
      where(users__id: u.uid)
    halt "no flatiron user found for IDENT #{ident}" if r.count != 1

    # SecureRandom has some issues, but should suffice for these purposes:
    # https://bugs.ruby-lang.org/issues/9569
    tokenstr = SecureRandom::base64(191)[0..254]
    h = {:user_id => u.uid, :token => tokenstr, :type => 'login', :scope => 'full'}
    begin
      NITROUS_DB[:tokens] << h
      puts "created token #{h}"
    rescue Sequel::DatabaseError => e
      puts "error creating token #{h}: #{e}"
    end
  end

  desc "allow_free IDENT", "Allows user IDENT to be a free user"
  def allow_free(ident)
    u = User.find(ident) {|ds| ds.where(users__deleted_at: nil) }
    halt "User #{u.username} can already create free workspaces" if u.can_create_free?
    halt "User #{u.username} is already a paying user: billing profile state (#{u.bp_state})" if u.bp_state != 'no_card'

    old_authed, old_bp_state = u.phone_authed, u.bp_state
    NITROUS_DB.transaction do
      r = NITROUS_DB[:users].where(id: u.uid).update(phone_authed: true)
      rows_expected!(r, 1)
      r = NITROUS_DB[:billing_profiles].where(user_id: u.uid).update(state: 'free')
      rows_expected!(r, 1)
    end

    puts "#{u.summary}: phone_authed #{old_authed} => true, bp_state #{old_bp_state} => 'free'"
  end

  desc "set_usage_quota IDENT SECONDS", "set usage quota for a user"
  def set_usage_quota(ident, usage_quota)
    NITROUS_DB.transaction do
      u = User.find(ident) {|ds| ds.where(users__deleted_at: nil )}
      old_usage_quota = u.usage_quota
      r = NITROUS_DB[:users].where(id: u.uid).update(usage_quota: usage_quota)
      rows_expected!(r, 1)
      puts "#{u.summary}: usage_quota #{old_usage_quota} => #{usage_quota}"
    end
  end

  desc "reset_quota IDENT", "reset the current quota period for a user"
  def reset_quota(ident)
    NITROUS_DB.transaction do
      u = User.find(ident) {|ds| ds.where(users__deleted_at: nil )}
      old_period = u.usage_quota_restored_at
      new_period = Time.now
      r = NITROUS_DB[:users].where(id: u.uid).update(usage_quota_restored_at: new_period)
      rows_expected!(r, 1)
      puts "#{u.summary}: usage_quota_restored_at #{old_period} => #{new_period}"
    end
  end
end

# vim:ft=ruby
