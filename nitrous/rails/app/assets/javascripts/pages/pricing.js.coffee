pages.Pricing =
  init: ->
    pages.activePageName = 'Pricing'

    # Get page hash
    # --------------------------------
    hash = $(location).attr('hash')

    # Pricing selection
    # --------------------------------
    standardPlan = 'standard'
    basicPlan = 'basic'
    proPlan = 'pro'
    freePlan = 'free'

    $('.selectable').click (event) ->
      event.preventDefault()
      hash = $(this).attr('data-slug')
      window.location.hash = hash unless window.location.hash == '#faq'
      if($(this).hasClass('active'))
        false
      else
        if($(this)).is('[data-slug="free"]')
          $('.paid').css('opacity','.5')
        else
          $('.paid').css('opacity','1')
        $('.selectable.active').removeClass('active')
        $(this).addClass('active')
        false

    setActiveDetails = (pricing) ->
      forFree = $(".pricing-details > .box:first-of-type")
      forPaid = $(".pricing-details > .box:last-of-type")

      if (pricing == "free")
        forPaid.removeClass("active")
        forFree.addClass("active")
      else
        forFree.removeClass("active")
        forPaid.addClass("active")

    setActivePricing = (pricing) ->
      $("[data-slug=#{pricing}]").click()
      # $('input[type="range"]').val(0)
      setActiveDetails(pricing)
      false

    # Check page hash and set default pricing
    # --------------------------------
    onHashChange = (hash) ->
      setActivePricing switch hash
        when "#free"
          freePlan
        when "#basic"
          basicPlan
        when "#standard"
          standardPlan
        when "#pro"
          proPlan
        else
          standardPlan

    onHashChange(hash)

    $(window).on 'hashchange', ->
      onHashChange(location.hash)

    # monthly/hourly toggler
    pricingHourly = false

    updatePricing = ->
      $t = $('.pricing-toggle')
      if pricingHourly
        $t.addClass('hourly')
        $('.plan-price .hourly').addClass('show')
        $('.plan-price .monthly').removeClass('show')
      else
        $t.removeClass('hourly')
        $('.plan-price .hourly').removeClass('show')
        $('.plan-price .monthly').addClass('show')

    $('.pricing-toggle .toggle-btn').click ->
      pricingHourly = !pricingHourly
      updatePricing()

    $('.pricing-toggle label.monthly').click ->
      pricingHourly = false
      updatePricing()

    $('.pricing-toggle label.hourly').click ->
      pricingHourly = true
      updatePricing()

    updatePricing()

    $('.plan-price').hover ->
      $(this).find('.hourly').addClass('show')
      $(this).find('.monthly').removeClass('show')
    , ->
      $(this).find('.monthly').addClass('show')
      $(this).find('.hourly').removeClass('show')

    # fetch plans
    plans = []

    updateNumbers = ->
      for plan, i in plans
        basePrice = plan.base_price / 100
        monthlyPrice = (plan.base_price + plan.price ) / 100
        hourlyPrice = plan.hourly_price / 1000
        $plan = $(".pricing-block li.selectable[data-index=#{i}]")
        $plan.find('.plan-price .monthly .price').text(monthlyPrice.toFixed(2))
        $plan.find('.plan-price .hourly .price').text(hourlyPrice.toFixed(3))
        $plan.find('.plan-price .hourly .base-price').text(basePrice.toFixed(2))
        $plan.find('.plan-price').css(opacity: 1)

    $.getJSON('/api/v0/plans').then (data) ->
      return unless data.plans
      pl = data.plans.sort (a, b) ->
        aPrice = a.price + a.base_price
        bPrice = b.price + b.base_price
        if aPrice > bPrice
          1
        else if aPrice < bPrice
          -1
        else
          0
      for p in pl
        if (p.price + p.base_price) > 0
          plans.push(p)
      updateNumbers()
