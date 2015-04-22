#= require ./model
attr = SM.attr

App.AdminFreeQueue = klass = App.Model.extend
  id: attr('number', serialize: false)
  userId: attr('number', as: 'user_id', serialize: false)
  containerTemplateSlug: attr('string', as: 'container_template_slug', serialize: false)
  containerName: attr('string', as: 'container_name', serialize: false)
  hostName: attr('string', as: 'host_name', serialize: false)
  regionSlug: attr('string', as: 'region_slug', serialize: false)
  username: attr('string', as: 'username', serialize: false)
  state: attr('string', as: 'state', serialize: false)
  name: attr('string', as: 'name', serialize: false)
  email: attr('string', as: 'email', serialize: false)
  phone: attr('string', as: 'phone', serialize: false)
  phoneCountryCode: attr('string', as: 'phone_country_code', serialize: false)
  phoneAuthed: attr('bool', as: 'phone_authed', serialize: false)
  emailConfirmed: attr('bool', as: 'email_confirmed', serialize: false)
  createdAt: attr('date', as: 'created_at', serialize: false)
  updatedAt: attr('date', as: 'updated_at', serialize: false)

  saving: attr('bool')

  approve: ->
    @_action("approved")

  reject: ->
    @_action("rejected")

  _action: (state)->
    @set 'saving', true

    new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        data: {expected_state: @get('state'), new_state: state}
        type: 'POST'
        url: "/api/v0/admin/free_queue/#{@get('id')}/action"
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      )
      .then((data) =>
        @set('saving', false)
        @set('state', state)
        resolve()
      , (err)=>
        @set('saving', false)
        reject(err?.responseJSON)
      )
