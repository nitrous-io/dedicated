pages.Pricing =
  init: ->
    pages.activePageName = 'Pricing'

    # Update total price
    # --------------------------------
    totalPrice = $("#total-price")
    updatePrice = ->
      p1 = parseFloat($("#p1").text().split("$")[1])
      p2 = parseFloat($("#p2").text().split("$")[1])
      total = parseFloat(p1 + p2).toFixed(2)
      totalPrice.text("$ " + total)

    updatePrice()


    # Get page hash
    # --------------------------------
    hash = $(location).attr('hash')

    # Pricing selection
    # --------------------------------
    standardPlan = 'standard'
    basicPlan = 'basic'
    proPlan = 'pro'
    freePlan = 'free'

    selectable = $('.selectable')
    selectable.click (event) ->
      event.preventDefault()
      hash = $(this).attr('data-slug')
      window.location.hash = hash
      if($(this).hasClass('active'))
        return false
      else
        if($(this)).is('[data-slug="free"]')
          $('.paid').css('opacity','.5')
        else
          $('.paid').css('opacity','1')
        $('.selectable.active').removeClass('active')
        $(this).addClass('active')
        return false

    setActivePricing = (pricing) ->
      $('[data-slug="'+pricing+'"]').click()
      $('input[type="range"]').val(0)
      return false


    # Custom range inputs
    # --------------------------------
    s = document.createElement('style')
    r = document.querySelectorAll('input[type=range]')
    prefs = [
      'webkit-slider-runnable'
      'moz-range'
    ]
    styles = []
    l = prefs.length
    n = r.length
    document.body.appendChild s

    getTrackStyleStr = (el) ->
      str = ''
      j = el.dataset.idx
      min = el.min or 0
      perc = if el.max then ~ ~(100 * (el.value - min) / (el.max - min)) else el.value
      val = perc + '% 100%'
      i = 0
      while i < l
        str += 'input[type=range][data-idx="' + j + '"]::-' + prefs[i] + '-track{background-size:' + val + '}'
        i++
      str

    i = 0
    while i < n
      styles.push ''
      r[i].addEventListener 'input', (->
        styles[@dataset.idx] = getTrackStyleStr(this)
        s.textContent = styles.join('')
        return
      ), false
      i++


    # Sticky
    # --------------------------------
    top = $('.sticky').offset().top
    height = $('.sticky').height()
    $(window).scroll (event) ->
      y = $(this).scrollTop()
      distance = (y - height) + 240
      if (y < 520)
        if (y >= (top))
          $('.sticky').css(
            '-webkit-transform': 'translate3d(0,' + distance + 'px,0)',
            '-ms-transform': 'translate3d(0,' + distance + 'px,0)',
            'transform': 'translate3d(0,' + distance + 'px,0)'
          )
        else
          $('.sticky').css(
            '-webkit-transform': 'translate3d(0,0,0)',
            '-ms-transform': 'translate3d(0,0,0)',
            'transform': 'translate3d(0,0,0)'
          )


    # Check page hash and set default pricing
    # --------------------------------
    if !(hash == "")
      if (hash == "#free")
        setActivePricing(freePlan)
        $('html, body').scrollTop(0)
        $('html, body').animate scrollTop: '+=500px' , 800
      if (hash == "#basic")
        setActivePricing(basicPlan)
      if (hash == "#standard")
        setActivePricing(standardPlan)
      if (hash == "#pro")
        setActivePricing(proPlan)
    else
      # Set default plan
      setActivePricing(standardPlan)
