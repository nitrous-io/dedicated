#= require ./org-members-mixin

allowedRoleMap =
  owner: ['owner', 'admin', 'member', 'limited']
  admin: ['admin', 'member', 'limited']
  member: []
  limited: []

App.OrgMembersComponent = Em.Component.extend App.OrgMembersMixin,
  layoutName: 'components/org/members'
  elementId: 'org-members'

  org: null
  loading: false
  saving: false

  newMemberUsername: null
  newMemberRole: 'member'

  allowedRoles: (->
    allowedRoleMap[@get 'user.role'] ? []
  ).property 'user.role'

  canAddMember: (->
    ['owner', 'admin'].contains(@get 'user.role')
  ).property 'user.role'

  didInsertElement: ->
    @_super()
    @fetchMembers()

  actions:
    add: ->
      @set 'saving', true
      username = @get 'newMemberUsername'
      role = @get 'newMemberRole'
      url = "/api/v0/orgs/#{@get('org.slug')}/members/#{username}"
      Em.RSVP.Promise.cast(Em.$.ajax
        type: 'PUT'
        url: url
        dataType: 'json'
        headers: { Authorization: "Bearer #{App.get 'token'}" }
        data: { role: role }
      ).then((data) =>
        if data.members
          App.OrgMember.load(@get('org.members'), data.members)
          @set 'newMemberUsername', null
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Could not add new member, please try again.")
      ).finally =>
        @set 'saving', false
