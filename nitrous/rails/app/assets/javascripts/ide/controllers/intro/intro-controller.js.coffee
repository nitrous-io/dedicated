App.IntroController = Em.Controller.extend
  quitIntro: ->
    page = 'boxes.index'

    # If a user hasn't created a box, then just go straight to the box new page
    if App.get('boxes.length') == 0
      page = 'boxes.new'

    App.get('router').transitionTo page
