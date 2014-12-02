#= require ./base-model

App.CollabInvite = klass = App.BaseModel.extend
  email: Em.attr()
  state: Em.attr()
  created_at : Em.attr(Date)

klass.reopenClass
  adapter: null
  rootKey: 'invite'

  # TODO: get rid of this
  fetch: (boxId, callback) ->
    $.ajax(
      type: 'GET'
      dataType: 'json'
      url: "/boxes/#{boxId}/invites"
    ).done((json) ->
      records = _.map json.invites, (attrs) ->
        record = klass.create()
        record.loadHash attrs
        record
      callback null, records
    ).fail (json) ->
      callback json
