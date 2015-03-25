pages.Shared =
  init: ->
    $.getJSON "/api/v0/app_settings", (settings) ->
      Mp.init(settings.mixpanel_token)
      if pages.activePageName
        Mp.track("Viewed #{pages.activePageName}")

    $('form.cta-get-started input').focus(->
      $(this).closest('form').addClass('focused')
    ).blur ->
      $(this).closest('form').removeClass('focused')

    $('form.cta-get-started').submit (event) ->
      event.preventDefault()
      url = '/app/#/signup'
      if email = $(event.target).find("[name='email']").val()
        url += "?email=#{encodeURIComponent(email)}"
      # set href instead of replace to preserve history (navigate instead of redirect)
      window.location.href = url

    # Material buttons
    # --------------------------------
    $('.ripple').on 'click', (event) ->
      event.preventDefault()
      $this = $(this)
      href = $this.attr('href')
      $div = $('<div/>')
      btnOffset = $(this).offset()
      xPos = event.pageX - btnOffset.left
      yPos = event.pageY - btnOffset.top
      $div.addClass 'ripple-effect'
      $ripple = $('.ripple-effect')
      $ripple.css 'height', $(this).height()
      $ripple.css 'width', $(this).height()
      $div.css(
        top: yPos - ($ripple.height() / 2)
        left: xPos - ($ripple.width() / 2)
        background: $(this).data('ripple-color')).appendTo $(this)

      window.setTimeout (->
        $div.remove()
        if $this.is('input[type="submit"]')
          $(this).parents("form").submit()
        else if href
          window.location = href
          false
      ), 985



