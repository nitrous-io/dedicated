class Container
  ATTRS = %i(cid slug name username
    host_state terminated_at host_slug image
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
    NITROUS_DB[:containers].
      select_all(:containers, :users, :hosts).
      select_more(:users__username___username).
      select_more(:hosts__slug___host_slug).
      select_more(:hosts__state___host_state).
      select_more(:containers__slug___slug).
      select_more(:containers__name___name).
      select_more(:containers__terminated_at___terminated_at).
      join(:users, id: :containers__user_id).
      join(:hosts, id: :containers__host_id)
  end

  def self.from_rows(dataset)
    dataset.map do |r|
      Container.new(r)
    end
  end
end
