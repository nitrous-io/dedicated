App.UserSettings = klass = Em.Object.extend
  id: null
  auto_close_brackets: null
  highlight_line: null
  keybindings: null
  match_brackets: null
  ruler_column: null
  show_line_numbers: null
  show_trailing_space: null
  soft_tabs: null
  tab_size: null
  theme: null
  wrap_lines: null

  save: ->
    new Em.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'POST',
        url: '/api/v0/user/settings',
        headers: { Authorization: "Bearer #{App.get 'token'}" },
        data: @getProperties(Ember.keys(@))
      ).done((retData, status, jqxhr) =>
        @setProperties(retData)
        resolve(retData)
      ).fail (jqxhr) ->
        reject(jqxhr)


klass.reopenClass
  fetch: ->
    promise = new Ember.RSVP.Promise (resolve, reject) =>
      Em.$.ajax(
        type: 'GET',
        url: '/api/v0/user/settings',
        headers: { Authorization: "Bearer #{App.get 'token'}" }
      ).done((data, status, jqxhr) ->
        resolve(klass.create(data.user_settings))
      ).fail (jqxhr) ->
        reject(jqxhr)
    promise
