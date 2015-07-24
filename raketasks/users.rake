namespace :users do
  FIND_ONE_DESC = "Specify one of USER_ID, USER_USERNAME, or USER_EMAIL"

  desc "Outputs information on user. #{FIND_ONE_DESC}"
  task :info do
    u = find_one.all[0]
    puts "ID: #{u[:uid]}"
    puts "Username: #{u[:username]}"
    puts "Email: #{u[:email]}"
    puts "Confirmed? #{!u[:confirmed_at].nil?}"
    puts "Allowed to create free hosts? #{u[:phone_authed] && u[:bp_state] == 'free'}"
    puts "Phone authed: #{u[:phone_authed]}"
    puts "Deleted? #{!u[:deleted_at].nil?}"
    puts "Banned?  #{!u[:banned_at].nil?}"
    puts "Billing profile state: #{u[:bp_state]}"
    puts "Subscription state: #{u[:sub_state]}"
  end

  desc "Allow user to create free workspace"
  task :allow_free do
    u = find_one {|ds| ds.where(users__deleted_at: nil) }.all[0]
    if u[:phone_authed] && u[:bp_state] == 'free'
      raise "User can already create free workspaces"
    end
    if u[:bp_state] != 'no_card'
      raise "User is already a paying user: billing profile state (#{u[:bp_state]})"
    end

    old_authed, old_bp_state = u[:phone_authed], u[:bp_state]
    DB.transaction do
      r = DB[:users].where(id: u[:uid]).update(phone_authed: true)
      rows_expected!(r, 1)
      r = DB[:billing_profiles].where(user_id: u[:uid]).update(state: 'free')
      rows_expected!(r, 1)
    end

    puts "User #{u[:uid]} #{u[:username]} <#{u[:email]}>:  phone_authed #{old_authed} => true, bp_state #{old_bp_state} => 'free'"
  end

  def rows_expected!(actual, expected)
    raise "Unexpected number of rows updated! Got #{actual}, Expected #{expected}" if actual != expected
  end

  # returns a dataset that can be further filtered
  # to retrieve the row, do: r = find_one.all[0]
  # Supports optional block to filter the result
  # If the result isn't a single row, returns an error
  def find_one(&block)
    finders = {
      username:  'USER_USERNAME',
      email:     'USER_EMAIL',
      users__id: 'USER_ID',
    }
    column, value, count = nil, nil, 0
    finders.each do |db_column, envvar|
      next if ENV[envvar].nil?
      count += 1
      raise "Please specify only one of #{finders.values.inspect}" if count > 1
      column = db_column
      value = ENV[envvar]
    end
    raise "No query values given" if column.nil?
    q = user_query.where(column => value)

    q = block.call(q) if block_given?

    raise "no user found" if q.count == 0
    if q.count > 1
      puts "#{q.count} users found"
      p q.all if q.count < 10
      raise "Bad result"
    end
    q
  end

  # left joins with everything for easy access
  # aliases:
  #   users.id => uid
  #   billing_profiles.state => bp_state
  #   subscriptions.state => sub_state
  #   hosts.state => host_state
  def user_query
    DB[:users].distinct(:users__id).
      select_all(:users, :billing_profiles, :hosts, :subscriptions).
      select_more(:users__id___uid).
      select_more(:billing_profiles__state___bp_state).
      select_more(:subscriptions__state___sub_state).
      select_more(:hosts__state___host_state).
      left_join(:billing_profiles, user_id: :id).
      left_join(:hosts, user_id: :id).
      left_join(:subscriptions, billing_profile_id: :billing_profiles__id)
  end
end
