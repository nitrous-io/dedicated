pages.Home =
  init: ->
    pages.activePageName = 'Home'


    $('#hero-email').focus ->
      $(this).parents('form').addClass('focused')

    $('#hero-email').blur ->
      $(this).parents('form').removeClass('focused')


    # Hero Logos
    patternEl = $('.logo-pattern')
    heroLogos = ('<span class="logo nitrous"></span><span class="logo github"></span><span class="logo python"></span><span class="logo node"></span><span class="logo ubuntu"></span><span class="logo rails"></span><span class="logo meteor"></span><span class="logo html5"></span><span class="logo wordpress"></span><span class="logo code"></span><span class="logo element"></span><span class="logo cloud"></span><span class="logo boxes"></span><span class="logo react"></span><span class="logo laravel"></span><span class="logo php"></span>')
    heroLogosTarget = $('.logo-pattern .inner')
    patternEl.css('opacity','0')

    $.fn.shuffle = ->
      j = undefined
      i = 0
      while i < @length
        j = Math.floor(Math.random() * @length)
        $(@[i]).before $(@[j])
        i++
      this

    for i in [0...18]
      elCol = $('<div class="col"></div>').attr('id','logo-col-'+i)
      elCol.appendTo(heroLogosTarget)
      elCol.append(heroLogos)
      $('#logo-col-'+i+' .logo').shuffle()

    setTimeout ->
      patternEl.css('opacity','1')
      patternEl.addClass('fadeIn')
    , 250

    # Webkit only
    if (navigator.userAgent.indexOf('AppleWebKit') != -1)
      heroLogosTarget.addClass('heroMatrix')


