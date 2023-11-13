using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class UpsertBayRequestModel
{
    [Required]
    public bool Default { get; set; }

    public string[] CallsignFilter { get; set; } = Array.Empty<string>();
}
