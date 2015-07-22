namespace :users do
  FIND_ONE_DESC = "Specify one of USER_ID, USER_USERNAME, or USER_EMAIL"

  desc "Outputs information on user. #{FIND_ONE_DESC}"
  task :info do
    users = find_one
    case users.count
    when 0
      raise "No user found"
    when 1
      u = users.all[0]
      puts "ID: #{u[:id]}"
      puts "Username: #{u[:username]}"
      puts "Email: #{u[:email]}"
      puts "Confirmed? #{!u[:confirmed_at].nil?}"
      puts "Allowed to create free hosts? #{u[:phone_authed] && u[:bp_state] == 'free'}"
      puts "Phone authed: #{u[:phone_authed]}"
      puts "Deleted? #{!u[:deleted_at].nil?}"
      puts "Banned?  #{!u[:banned_at].nil?}"
      puts "Billing profile state: #{u[:bp_state]}"
      puts "Subscription state: #{u[:sub_state]}"
    else
      p "#{users.count} users returned"
      p users.all
      raise "Bad result"
    end
  end

  # returns a dataset that can be further filtered
  def find_one
    finders = {
      :username => 'USER_USERNAME',
      :email => 'USER_EMAIL',
      :users__id => 'USER_ID',
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
    user_query.where(column => value)
  end

  # left joins with everything for easy access
  # aliases:
  #   billing_profiles.state => bp_state
  #   subscriptions.state => sub_state
  #   hosts.state => host_state
  def user_query
    DB[:users].select_all(:users, :billing_profiles, :hosts, :subscriptions).
      select_more(:billing_profiles__state___bp_state).
      select_more(:subscriptions__state___sub_state).
      select_more(:hosts__state___host_state).
      left_join(:billing_profiles, :user_id => :id).
      left_join(:hosts, :user_id => :id).
      left_join(:subscriptions, :billing_profile_id => :billing_profiles__id)
  end
end
