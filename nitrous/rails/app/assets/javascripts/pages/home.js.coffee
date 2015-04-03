pages.Home =
  init: ->
    pages.activePageName = 'Home'

    # Hero Logos
    $pattern = $('.logo-pattern')
    $patternInner = $('.logo-pattern .inner')

    window.setTimeout ->
      $pattern.css(opacity: 1)
      $pattern.addClass('fadeIn')
    , 250

    # Webkit only
    if (navigator.userAgent.indexOf('AppleWebKit') != -1)
      $patternInner.addClass('heroMatrix')
