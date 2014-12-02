#= require jquery
window.App ||= {}

App.HackPage =
  initialize: ->
    @i = 0
    @steps = $("ul.hack-steps li")

    setInterval(=>
     this.changeStep()
    , 3800)

  changeStep: ->
    $(@steps[(@i % @steps.length)]).removeClass 'active'
    @i += 1
    $(@steps[(@i % @steps.length)]).addClass 'active'

  didDragBookMarklet: ->
    App.util.mixpanel 'Bookmarklet Installed'
