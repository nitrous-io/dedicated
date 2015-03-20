HERO_LOGOS = 'nitrous github python node ubuntu rails meteor html5 wordpress code element cloud boxes react laravel php'

pages.Home =
  init: ->
    pages.activePageName = 'Home'

    # Hero Logos
    $pattern = $('.logo-pattern')
    heroLogos = HERO_LOGOS.split(' ').map((o) ->
      "<span class='logo #{o}'></span>"
    ).join('')
    $heroLogosTarget = $('.logo-pattern .inner')
    $pattern.css(opacity: 0)

    $.fn.shuffle = ->
      j = undefined
      i = 0
      while i < @length
        j = Math.floor(Math.random() * @length)
        $(@[i]).before $(@[j])
        i++
      this

    for i in [0...18]
      $col = $('<div class="col"></div>').attr('id',"logo-col-#{i}")
      $col.appendTo($heroLogosTarget)
      $col.append($(heroLogos))
      $("#logo-col-#{i} .logo").shuffle()

    window.setTimeout ->
      $pattern.css(opacity: 1)
      $pattern.addClass('fadeIn')
    , 250

    # Webkit only
    if (navigator.userAgent.indexOf('AppleWebKit') != -1)
      $heroLogosTarget.addClass('heroMatrix')
