App.IdeView = Em.View.extend
  templateName: 'ide/ide'
  elementId: 'ide-view'

  willInsertElement: ->
    @_super()
    $('html').addClass 'ide'
    $(window).bind 'focus.ideView', ->
      App.get('router.ideEditorController').focusActiveTab()

    # prevent accidental backspacing
    $(window).bind 'keydown.ideView', (e) ->
      if e.keyCode == 8 && $(e.target).is('body')
        e.preventDefault()

  didInsertElement: ->
    @_super()
    controller = @get 'controller'
    controller.validateContainerState()
    mp.track 'IDE Session Started', container_id: @get 'controller.id'
    mp.increment 'ide_session_started_count'
    Em.run.scheduleOnce 'afterRender', ->
      webapp?.loading.stop()

  willDestroyElement: ->
    @_super()
    $('html').removeClass 'ide'
    $(window).unbind 'focus.ideView'
    $(window).unbind 'keydown.ideView'
