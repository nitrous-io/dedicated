App.OrgMembersMixin = Em.Mixin.create
  org: null

  user: (->
    userId = App.get 'user.id'
    @get('org.members')?.findBy('userId', userId)
  ).property 'org.members.@each', 'App.user.id'

  fetchMembers: (->
    return unless org = @get 'org'
    @set 'loading', true
    org.fetchMembers().catch((jqxhr) =>
      App.postError(jqxhr.responseJSON, "Could not load members for “#{org.get 'displayName'}”, please try again.")
    ).finally =>
      @set 'loading', false
  ).observes 'org'
