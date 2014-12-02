App.BoxProvisioningView = Em.View.extend
  templateName: 'templates/box/provisioning'
  classNames: ['app_page']
  elementId: 'box_provisioning_view'

  title: 'Provisioning'
  titleInterval: null

  didInsertElement: ->
    @_super()

    titleCounter = 0
    @set 'titleInterval', window.setInterval(=>
      @set 'title', 'Provisioning' + '...'.substring(0, ++titleCounter % 4)
    , 1000)

    $animatedGauge = @$('#animated-gauge')
    window.setTimeout(->
      $animatedGauge.addClass('reset')
      window.setTimeout(->
        $animatedGauge.removeClass('reset').addClass('charging')
        window.setTimeout(->
          $animatedGauge.removeClass('charging').addClass('shaking')
        , 8000)
      , 2000)
    , 1000)

  humanizedState: (->
    if (boxState = @get('controller.state')) == 'cloning_repo'
      "downloading repo into"
    else
      boxState
  ).property 'controller.state'

  willDestroyElement: ->
    @_super()
    window.clearInterval @get 'titleInterval'
    @set 'titleInterval', null
