#= require ./box-ide-view

App.BoxTerminalView = App.BoxIdeView.extend
  templateName: 'templates/box/terminal'
  elementId: 'ide_terminal_view'
  classNames: 'ide_view'

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Web Terminal Started', box_id: @get 'controller.id'
    App.util.mixpanelIncrement 'web_terminal_started_count'

