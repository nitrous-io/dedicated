class Project
  ATTRS = %i(
    email project_name domain_name deployed_at
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
      if col == :project_name && val != '<null>'
        val = "#{val} (http://#{val}.pubstorm.site)"
      end
      "#{col}: #{val}"
    end
    lines.join("\n")
  end

  def self.query
    PUBSTORM_DB[:projects].
      select(:users__email, :projects__name___project_name,
             :domains__name___domain_name, :deployments__deployed_at).
      join_table(:left, :domains, project_id: :id).
      join(:users, id: :projects__user_id).
      join(:deployments, project_id: :projects__id).
      where(projects__deleted_at: nil)
  end

  def self.from_rows(dataset)
    dataset.map do |r|
      Project.new(r)
    end
  end
end
