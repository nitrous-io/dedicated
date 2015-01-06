#= require ../mixins/terminal-connection-mixin

App.ContainerConsoleController = Em.ObjectController.extend App.TerminalConnectionMixin,
  queryParams: ['action', 'fullscreen']
  action: null
  fullscreen: false

  record: Em.computed.alias 'model'
  ready: Em.computed.and 'freighterdUrl', 'record.config'

  makeUrlAndConnect: (terminal) ->
    return unless container = @get 'record'
    action = @get('action')

    @makeUrl(container, terminal, action).then (url) ->
      terminal.url = url
      terminal.open(true)
      terminal.focus()
    , (jqxhr) ->
      terminal.retry()
      App.postError(jqxhr.responseJSON, 'Could not connect to shell, retrying...')
