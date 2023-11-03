using System.ComponentModel.DataAnnotations;
using Microsoft.EntityFrameworkCore;

namespace Vatsim.Scandinavia.FlightStrips.Persistence.EfCore.Entities;

[PrimaryKey(nameof(Name), nameof(Airport))]
public class BayEntity : IAirportTenant
{
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
