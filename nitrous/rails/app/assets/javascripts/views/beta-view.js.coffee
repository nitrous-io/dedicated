App.BetaView = Em.View.extend
  elementId: 'beta-view'
  templateName: 'beta'

  closeFeedbackDidChange: (->
    if @get 'controller.closeFeedback'
      @set 'controller.closeFeedback', false
      @send 'cancelFeedback'
  ).observes 'controller.closeFeedback'

  closeSwitchUserDidChange: (->
    if @get 'controller.closeSwitchUser'
      @set 'controller.closeSwitchUser', false
      @send 'cancelSwitchUser'
  ).observes 'controller.closeSwitchUser'

  didInsertElement: ->
    @_super()
    App.on 'open-feedback-dialog', ->
      ui.showModal(@$('#feedback-dialog'))
      @$('#feedback-subject').focus()

    App.on 'open-switch-user-dialog', ->
      ui.showModal(@$('#switch-user-dialog'))
      @$('#switch-user-login').focus()

  willDestroyElement: ->
    @_super()
    App.off 'open-feedback-dialog'
    App.off 'open-switch-user-dialog'

  actions:
    cancelFeedback: ->
      ui.hideModal(@$('#feedback-dialog'))

    cancelSwitchUser: ->
      ui.hideModal(@$('#switch-user-dialog'))
