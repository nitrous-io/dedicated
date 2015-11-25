pages.Pricing =
  init: ->
    pages.activePageName = 'Pricing'

    # fetch plans
    plans = []

    updateNumbers = ->
      for plan, i in plans
        monthlyPrice = (plan.base_price + plan.price) / 100
        priceToDisplay = if monthlyPrice == Math.floor(monthlyPrice) then Math.floor(monthlyPrice) else monthlyPrice.toFixed(2)

        $plan = $(".pricing-block li.selectable[data-index=#{i}]")
        $plan.find('.plan-price .monthly .price').text(priceToDisplay)
        $plan.find('.title').text(plan.name)
        $plan.find('.desc span').text(plan.description)
        $plan.find('.collaborators strong').text(if plan.max_collaborators then plan.max_collaborators.toString() else 'Unlimited')
        $plan.find('.containers strong').text(plan.max_containers.toString())
        $plan.find('.memory strong').text(plan.memory)
        $plan.find('.storage strong').text(plan.storage)

        if plan.trial_period_days > 0
          $plan.find('a.btn').text("#{plan.trial_period_days}-day Free Trial")

      $('.plan-list').css(opacity: 1)

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
        if (p.price + p.base_price) > 0 && !p.org && !p.hidden
          plans.push(p)
      for p in pl
        if (p.price + p.base_price) > 0 && p.org && !p.hidden
          p.name = "Org"
          p.price = p.price / p.max_hosts;
          p.description = "Minimum purchase of #{p.max_hosts} workstations required"
          plans.push(p)
      updateNumbers()
