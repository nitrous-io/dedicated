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
    plusPlan = 'plus'
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
        when "#plus"
          plusPlan
        else
          standardPlan

    onHashChange(hash)

    $(window).on 'hashchange', ->
      onHashChange(location.hash)

    # fetch plans
    plans = []

    updateNumbers = ->
      for plan, i in plans
        monthlyPrice = (plan.base_price + plan.price) / 100
        priceToDisplay = if monthlyPrice == Math.floor(monthlyPrice) then Math.floor(monthlyPrice) else monthlyPrice.toFixed(2)

        $plan = $(".pricing-block li.selectable[data-index=#{i}]")
        $plan.find('.plan-price .monthly .price').text(priceToDisplay)

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
