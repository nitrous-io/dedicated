class User
  ATTRS = %i(uid username email confirmed_at phone_authed
    deleted_at banned_at bp_state sub_state)

  attr_reader *ATTRS

  # used like User.query.map {|u| User.new(u) }
  def initialize(query_row)
    ATTRS.each do |s|
      self.instance_variable_set("@#{s}", query_row[s])
    end
  end

  def to_s
    lines = ATTRS.map do |s|
      val = send(s)
      val = val.nil? ? "<null>" : val
      "#{s}: #{val}"
    end

    lines << "Can create free? #{phone_authed && bp_state == 'free'}"
    
    lines.join("\n")
  end

  # Returns a single user object
  # Supports optional block to filter the result
  # If the result isn't a single row, returns an error
  def self.find_one(&block)
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
    q = User.query.where(column => value)

    q = block.call(q) if block_given?

    raise "no user found" if q.count == 0
    if q.count > 1
      puts "#{q.count} users found"
      p q.all if q.count < 10
      raise "Bad result"
    end
    self.new(q.all[0])
  end

  # left joins with everything for easy access
  # aliases:
  #   users.id => uid
  #   billing_profiles.state => bp_state
  #   subscriptions.state => sub_state
  #   hosts.state => host_state
  def self.query
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
