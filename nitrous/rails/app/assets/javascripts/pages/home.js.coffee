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

    $("#get-started").submit (event) ->
      event.preventDefault()
      url = '/app/#/signup'
      if email = $(event.target).find("[name='email']").val()
        url += "?email=#{encodeURIComponent(email)}"
      # set href instead of replace to preserve history (navigate instead of redirect)
      window.location.href = url
