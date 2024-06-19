using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Models;

public class StripClearRequestModel
{
    [Required]
    public bool IsCleared { get; set; }

    [Required]
    public required string Position { get; set; }

}
