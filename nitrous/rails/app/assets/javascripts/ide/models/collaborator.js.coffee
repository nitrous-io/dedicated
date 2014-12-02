#= require ./base-model

App.Collaborator = klass = App.BaseModel.extend
  username: Em.attr()
  full_name: Em.attr()
  gravatar_hash: Em.attr()
  created_at: Em.attr(Date)

  removing: false

  gravatarUrl: (->
    if gravatar_hash = @get 'gravatar_hash'
      "https://secure.gravatar.com/avatar/#{gravatar_hash}.png?s=25"
  ).property 'gravatar_hash'

  remove: (boxId, callback) ->
    @set 'removing', true
    $.ajax(
      type: 'DELETE'
      dataType: 'json'
      url: "/boxes/#{boxId}/collaborators/#{@get 'username'}"
    ).done((json) ->
      callback null
    ).fail((json) ->
      callback json
    ).always =>
      @set 'removing', false

klass.reopenClass
  adapter: null
  rootKey: 'collaborator'

  # TODO: get rid of these
  fetch: (boxId, callback) ->
    klass = this
    $.ajax(
      type: 'GET'
      dataType: 'json'
      url: "/boxes/#{boxId}/collaborators"
    ).done((json) ->
      records = _.map json.collaborators, (attrs) ->
        record = klass.create()
        record.loadHash attrs
        record
      callback null, records
    ).fail (json) ->
      callback json

  notifyMention: (mention, boxId) ->
    $.ajax
      type: 'POST'
      dataType: 'json'
      url: "/boxes/#{boxId}/collaborators/#{mention}/notify"
