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
      # $('input[type="range"]').val(0)
      return false


    # Check page hash and set default pricing
    # --------------------------------
    if !(hash == "")
      if (hash == "#free")
        setActivePricing(freePlan)
      if (hash == "#basic")
        setActivePricing(basicPlan)
      if (hash == "#standard")
        setActivePricing(standardPlan)
      if (hash == "#pro")
        setActivePricing(proPlan)
    else
      # Set default plan
      setActivePricing(standardPlan)

    # monthly/hourly toggler
    pricingHourly = false

    updatePricing = ->
      $t = $('.pricing-toggle')
      if pricingHourly
        $t.addClass('hourly')
        $('.plan-price .hourly').css(opacity: 1)
        $('.plan-price .monthly').css(opacity: 0)
      else
        $t.removeClass('hourly')
        $('.plan-price .hourly').css(opacity: 0)
        $('.plan-price .monthly').css(opacity: 1)

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
