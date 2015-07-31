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

    lines << "Can create free? #{can_create_free?}"

    lines.join("\n")
  end

  def can_create_free?
    phone_authed && bp_state == 'free'
  end

  # Returns a single user object
  # Supports optional block to filter the result
  # If the result isn't a single row, returns an error
  # ident can be id, username, or e-mail
  # id = all numeric
  # e-mail = has '@'
  # username = everything else
  # all numeric usernames are possible but unsupported
  def self.find(ident, &block)
    column, value = case ident
    when /\A\d+\z/
      [:users__id, ident.to_i]
    when /@/
      [:email, ident]
    else
      [:username, ident]
    end

    q = User.query.where(column => value)
    q = block.call(q) if block_given?

    halt "no user found" if q.count == 0
    if q.count > 1
      puts "#{q.count} users found"
      halt "Bad result"
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
