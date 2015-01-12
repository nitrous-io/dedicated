pages.Home =
  init: ->
    $ ->
      screens = ['ide', 'main', 'desktop']
      i = 0
      transition = ->
        screen = screens[i]
        $(".overview").addClass('fadeout')
        $(".overview.#{screen}").removeClass('fadeout')
        i = (i + 1) % 3

      window.setInterval(transition, 6000)
      transition()
