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
  font_size: null

  requestSave: (delay = 5000)->
    return if @saveTimer
    @saveRequested = true
    @saveTimer = window.setTimeout(=>
      @saveTimer = null
      @save()
      @saveRequested = false
    , delay)

  save: ->
    return unless @saveRequested
    Em.$.ajax(
      type: 'POST',
      url: '/api/v0/user/settings',
      contentType : "application/json",
      dataType: "json",
      headers: { Authorization: "Bearer #{App.get 'token'}" },
      data: JSON.stringify(@getProperties(Ember.keys(@)))
    ).done((retData, status, jqxhr) =>
      @setProperties(retData)
    ).fail (jqxhr) =>
      @requestSave(1000)


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
