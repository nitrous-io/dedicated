App.TerminalView = Em.View.extend
  templateName: 'ide/terminal'
  elementId: 'ide-view'

  willInsertElement: ->
    @_super()
    $('html').addClass 'terminal'

  didInsertElement: ->
    @_super()
    controller = @get 'controller'
    controller.validateContainerState()
    Em.run.scheduleOnce 'afterRender', ->
      webapp.loading.stop()

  willDestroyElement: ->
    @_super()
    $('html').removeClass 'terminal'
