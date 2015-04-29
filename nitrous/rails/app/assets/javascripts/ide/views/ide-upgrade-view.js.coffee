App.IdeUpgradeView = Em.View.extend
  templateName: 'ide/upgrade'
  elementId: 'ide-upgrade'

  willInsertElement: ->
    @_super()
    $('html').addClass 'ide'

    # prevent accidental backspacing
    $(window).bind 'keydown.ideView', (e) ->
      if e.keyCode == 8 && $(e.target).is('body')
        e.preventDefault()

  didInsertElement: ->
    @_super()
    controller = @get 'controller'
    #controller.validateContainerState()
    Em.run.scheduleOnce 'afterRender', ->
      webapp?.loading.stop()

  willDestroyElement: ->
    @_super()
    $('html').removeClass 'ide'
    $(window).unbind 'keydown.ideView'
