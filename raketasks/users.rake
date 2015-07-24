namespace :users do
  FIND_ONE_DESC = "Specify one of USER_ID, USER_USERNAME, or USER_EMAIL"

  desc "Outputs information on user. #{FIND_ONE_DESC}"
  task :info do
    u = User.find_one
    puts u
    # puts "Allowed to create free hosts? #{u.phone_authed && u.bp_state == 'free'}"
  end

  desc "Allow user to create free workspace"
  task :allow_free do
    u = User.find_one {|ds| ds.where(users__deleted_at: nil) }
    if u.phone_authed && u.bp_state == 'free'
      raise "User can already create free workspaces"
    end
    if u.bp_state != 'no_card'
      raise "User is already a paying user: billing profile state (#{u.bp_state})"
    end

    old_authed, old_bp_state = u.phone_authed, u.bp_state
    DB.transaction do
      r = DB[:users].where(id: u.uid).update(phone_authed: true)
      rows_expected!(r, 1)
      r = DB[:billing_profiles].where(user_id: u.uid).update(state: 'free')
      rows_expected!(r, 1)
    end

    puts "User #{u.uid} #{u.username} <#{u.email}>:  phone_authed #{old_authed} => true, bp_state #{old_bp_state} => 'free'"
  end

  def rows_expected!(actual, expected)
    raise "Unexpected number of rows updated! Got #{actual}, Expected #{expected}" if actual != expected
  end

end
