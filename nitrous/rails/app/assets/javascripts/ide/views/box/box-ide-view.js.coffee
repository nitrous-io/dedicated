App.BoxIdeView = Em.View.extend
  templateName: 'templates/box/ide'
  elementId: 'ide_view'
  classNames: 'ide_view'

  didClickMacAnnouncement: ->
    App.localStorage.set('clickedMacAnnouncement', true)
    $('.top-announcement').remove()
    window.open("/mac", "_blank")

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
    App.util.mixpanel 'IDE Session Started', box_id: @get 'controller.id'
    App.util.mixpanelIncrement 'ide_session_started_count'

  willDestroyElement: ->
    @_super()
    $('html').removeClass 'ide'
    $(window).unbind 'focus.ideView'
    $(window).unbind 'keydown.ideView'

  adjustTopPaddingForMainMenu: (->
    mainMenuViewHeight = App.get 'router.ideLayoutController.mainMenuViewHeight'
    @$().css 'padding-top', mainMenuViewHeight
  ).observes 'App.router.ideLayoutController.mainMenuViewHeight'

