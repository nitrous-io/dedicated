App.HostIndexController = Em.ObjectController.extend
  performAction: (method, action, tempState) ->
    host = @get('model')
    prevState = host.get('state')
    host.set 'state', tempState
    ajax = host.ajax(method, action).then (json) ->
      host.load(json.host) if json.host?
    , (jqxhr) ->
      if errMsg = jqxhr.responseJSON?.error_description
        App.snackbar.post(errMsg.capitalize())
      host.set('state', prevState)
    ajax

  actions:
    start: ->
      @performAction('POST', 'start', 'starting')

    restart: ->
      @performAction('POST', 'restart', 'restarting')

    stop: ->
      @performAction('POST', 'stop', 'stopping')

    terminate: ->
      console.log 'terminate'

    refresh: ->
      @get('model').fetchStats()
