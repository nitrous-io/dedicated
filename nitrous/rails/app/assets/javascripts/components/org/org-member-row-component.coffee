App.OrgMemberRowComponent = Em.Component.extend
  layoutName: 'components/org/member-row'
  tagName: 'li'
  classNames: ['org-member-row', 'list-item']

  member: null
  user: null
  org: null
  allowedRoles: null

  newRole: null

  saving: false

  isSelf: (->
    @get('user') == @get('member')
  ).property 'user', 'member'

  canEdit: (->
    role = @get 'member.role'
    @get('allowedRoles')?.contains(role) && !@get('isSelf')
  ).property 'member.role', 'allowedRoles', 'isSelf'

  didInsertElement: ->
    @_super()
    @set 'newRole', @get('member.role')

  roleUpdated: (->
    newRole = @get 'newRole'
    return if @get('member.role') == newRole
    @set 'saving', true
    username = @get 'member.username'
    url = "/api/v0/orgs/#{@get('org.slug')}/members/#{username}"
    Em.RSVP.Promise.cast(Em.$.ajax
      type: 'PUT'
      url: url
      dataType: 'json'
      headers: { Authorization: "Bearer #{App.get 'token'}" }
      data: { role: newRole }
    ).then((data) =>
      if data.members
        App.OrgMember.load(@get('org.members'), data.members)
    , (jqxhr) ->
      App.postError(jqxhr.responseJSON, "Could not update role of “#{username}”, please try again.")
    ).finally =>
      @set 'saving', false
  ).observes 'newRole'

  actions:
    showRemoveMember: ->
      ui.showModal '.remove-member-modal-dialog'

    hideRemoveMember: ->
      ui.hideModal '.remove-member-modal-dialog'

    removeMember: ->
      ui.hideModal '.remove-member-modal-dialog'
      username = @get('member.username')

      @set 'saving', true
      url = "/api/v0/orgs/#{@get('org.slug')}/members/#{username}"
      Em.RSVP.Promise.cast(Em.$.ajax
        type: 'DELETE'
        url: url
        dataType: 'json'
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).then((data) =>
        if data.members
          App.OrgMember.load(@get('org.members'), data.members)
        App.snackbar.post("You removed “#{username}“")
      , (jqxhr) ->
        App.postError(jqxhr.responseJSON, "Could not remove “#{username}“, please try again.")
      ).finally =>
        @set 'saving', false
