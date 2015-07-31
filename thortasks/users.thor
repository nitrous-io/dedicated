class Users < Thor
  desc "info IDENT",  "Outputs information on single user, where IDENT is id, username, or email"
  def info(ident)
    puts User.find(ident)
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

  desc "allow_free IDENT", "Allows user IDENT to be a free user"
  def allow_free(ident)
    u = User.find(ident) {|ds| ds.where(users__deleted_at: nil) }
    halt "User #{u.username} can already create free workspaces" if u.can_create_free?
    halt "User #{u.username} is already a paying user: billing profile state (#{u.bp_state})" if u.bp_state != 'no_card'

    old_authed, old_bp_state = u.phone_authed, u.bp_state
    DB.transaction do
      r = DB[:users].where(id: u.uid).update(phone_authed: true)
      rows_expected!(r, 1)
      r = DB[:billing_profiles].where(user_id: u.uid).update(state: 'free')
      rows_expected!(r, 1)
    end

    puts "User #{u.uid} #{u.username} <#{u.email}>:  phone_authed #{old_authed} => true, bp_state #{old_bp_state} => 'free'"
  end
end

# vim:ft=ruby
