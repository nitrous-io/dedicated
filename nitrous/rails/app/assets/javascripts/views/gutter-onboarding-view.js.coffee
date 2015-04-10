App.GutterOnboardingView = Em.View.extend
  elementId: 'gutter-onboarding-view'
  templateName: 'gutter-onboarding'

  parentView: null

  screenBinding: 'controller.screen'
  nextScreenBinding: 'controller.nextScreen'

  showPickContainer: Em.computed.equal 'screen', 'pick-container'
  showSelectContainerName: Em.computed.equal 'screen', 'select-container-name'
  showSelectRegion: Em.computed.equal 'screen', 'select-region'
  showSelectPlan: Em.computed.equal 'screen', 'select-plan'
  showCardholdersName: Em.computed.equal 'screen', 'cardholders-name'

  showNextButton: (->
    @get('nextScreen')?.indexOf('waiting-') == -1
  ).property 'nextScreen'

  didInsertElement: ->
    @_super()
    $('#onboarding-view').on('DOMNodeInserted', @positionCards)
    $(window).on('resize', @positionCards)

  positionCards: ->
    $("#gutter-onboarding-view .card").each (index, card)->
      card = $(card)
      target = card.data("target")
      unless target
        return

      target = $(target)
      pos = target.offset()
      return unless pos
      content = $("#content-wrapper")
      left = content.offset().left - card.width()
      middle = (pos.top + (target.height()/2)) - 25
      card.offset({top: middle, left: left})
      setTimeout ->
        card.addClass "animated"
      , 5
