pages.Home =
  init: ->
    pages.activePageName = 'Home'

    # Hero Logos
    $pattern = $('.logo-pattern')
    $patternInner = $('.logo-pattern .inner')

    # Notice Banner
    $noticeBanner = $(".notice-banner")
    $noticeBannerClose = $(".notice-banner .close")

    window.setTimeout ->
      $pattern.css(opacity: 1)
      $pattern.addClass('fadeIn')
    , 250

    if window.location.search == '?l=1'
      # users redirected from www.nitrous.io will have this querystring
      window.setTimeout ->
        $(".notice-banner").addClass('visible')
      , 920

    # Webkit only
    #if (navigator.userAgent.indexOf('AppleWebKit') != -1)
      #$patternInner.addClass('heroMatrix')

    $noticeBannerClose.click ->
      $noticeBanner.removeClass("visible")
      return false
