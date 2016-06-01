class Host
  ATTRS = %i(hid slug username
    state terminated_at region_identifier provider_identifier provider_storage_identifier
    plan_name subscription_state
  )

  attr_reader *ATTRS

  def initialize(query_row)
    ATTRS.each do |col, _|
      self.instance_variable_set("@#{col}", query_row[col])
    end
  end

  def to_s
    lines = ATTRS.map do |col|
      val = send(col)
      val = val.nil? ? "<null>" : val
      "#{col}: #{val}"
    end
    lines.join("\n")
  end

  def self.query
    NITROUS_DB[:hosts].
      select_all(:hosts, :plans, :regions).
      select_more(:users__username___username).
      select_more(:hosts__slug___slug).
      select_more(:hosts__id___hid).
      select_more(:plans__name___plan_name).
      select_more(:subscriptions__state___subscription_state).
      join(:subscriptions, id: :subscription_id).
      join(:users, id: :hosts__user_id).
      join(:plans, id: :hosts__plan_id).
      join(:regions, id: :hosts__region_id)
  end

  def self.from_rows(dataset)
    dataset.map do |r|
      Host.new(r)
    end
  end
end
