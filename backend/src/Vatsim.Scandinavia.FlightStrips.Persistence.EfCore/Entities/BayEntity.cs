using System.ComponentModel.DataAnnotations;
using Microsoft.EntityFrameworkCore;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

public class BayEntity : IAirportTenant
{
    public int Id { get; set; }

    public string Airport { get; set; } = string.Empty;

    public required string Name { get; set; }

    public bool Default { get; set; }

    public ICollection<BayFilter> Filters { get; set; } = new List<BayFilter>();

}

[Owned]
public class BayFilter
{
    [Key]
    public required string Callsign { get; set; }
}
