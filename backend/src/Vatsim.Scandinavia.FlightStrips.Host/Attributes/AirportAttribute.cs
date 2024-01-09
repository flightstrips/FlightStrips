using System.ComponentModel.DataAnnotations;

namespace Vatsim.Scandinavia.FlightStrips.Host.Attributes;

public class AirportAttribute() : RegularExpressionAttribute("^[A-z]{4}$");
