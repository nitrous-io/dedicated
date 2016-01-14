pages.Pricing =
  init: ->
    pages.activePageName = 'Pricing'

    updateNumbers = (plans) ->
      for plan, i in plans
        monthlyPrice = (plan.base_price + plan.price) / 100
        priceToDisplay = if monthlyPrice == Math.floor(monthlyPrice) then Math.floor(monthlyPrice) else monthlyPrice.toFixed(2)

        $plan = $(".pricing-block li.selectable[data-index=#{i}]")
        $plan.find('.plan-price .monthly .price').text(priceToDisplay)
        $plan.find('.title').text(plan.name)
        $plan.find('.desc span').text(plan.description)
        $plan.find('.containers strong').text(plan.max_containers.toString())
        if plan.max_containers == 1
          $plan.find('.containers span').text('Project')
        if plan.max_collaborators
          $plan.find('.collaborators strong').text(plan.max_collaborators.toString())
          if plan.max_collaborators == 1
            $plan.find('.collaborators span').text('collaborator')
        else
          $plan.find('.collaborators strong').text('Unlimited')
        $plan.find('.memory strong').text(plan.memory)
        $plan.find('.storage strong').text(plan.storage)

      $('.plan-list').css(opacity: 1)

    # FIXME This is very fragile.
    $.getJSON('/api/v0/plans/all').then (data) ->
      return unless data.plans
      data.plans.sort (a, b) ->
        aPrice = a.price + a.base_price
        bPrice = b.price + b.base_price
        aPrice - bPrice

      plans = []
      for p in data.plans
        if p.edu || p.org || p.hidden
          continue

        plans.push(p)

      # Insert org plan(s) at the end.
      for p in data.plans
        if p.hidden || p.edu || !p.org || !p.max_hosts || p.max_hosts <= 0
          continue

        if (p.price + p.base_price) > 0
          p.name = "Org"
          p.price = p.price / p.max_hosts
          p.description = "Minimum purchase of #{p.max_hosts} workstations required"
          plans.push(p)

      updateNumbers(plans)
