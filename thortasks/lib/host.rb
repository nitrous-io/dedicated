class Host
  ATTRS = %i(id slug state terminated_at username provider_identifier provider_storage_identifier)

  # maps db column to output label
  ALIASED = {
    :provider_identifier => :instance_id,
    :provider_storage_identifier => :volume_id,
  }

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
      label = ALIASED[col] || col.to_s
      "#{label}: #{val}"
    end
    lines.join("\n")
  end

  def self.query
    DB[:hosts].
      select_all(:hosts).
      select_more(:users__username___username).
      join(:users, id: :user_id)
  end

  def self.from_rows(dataset)
    dataset.map do |r|
      Host.new(r)
    end
  end
end
